// Package service holds application use cases that coordinate the device
// repository, network resolution, the WoL engine and availability checks.
package service

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"wakepilot/internal/device"
)

// Availability states. "unreachable" means no probe was answered; it does NOT
// mean the device is powered off (firewalls commonly drop probes).
const (
	Reachable   = "reachable"
	Unreachable = "unreachable"
	Unknown     = "unknown" // never checked, or no IP configured
)

type Availability struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checkedAt"`
}

// Checker probes a host. Implementations must respect ctx.
type Checker interface {
	// Check returns whether ip answered. An error means the check itself
	// could not be performed (e.g. cancelled), not that the host is down.
	Check(ctx context.Context, ip string) (bool, error)
}

// TCPChecker treats a host as reachable when any of Ports accepts a TCP
// connection or actively refuses it (a refusal proves a live TCP stack).
// ICMP is deliberately not used: it needs raw-socket privileges.
type TCPChecker struct {
	Ports   []int
	Timeout time.Duration
}

func (c TCPChecker) Check(ctx context.Context, ip string) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan bool, len(c.Ports))
	for _, p := range c.Ports {
		go func(port int) {
			d := net.Dialer{Timeout: c.Timeout}
			conn, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip, strconv.Itoa(port)))
			if err == nil {
				conn.Close()
			}
			results <- err == nil || isRefused(err)
		}(p)
	}
	for range c.Ports {
		if <-results {
			return true, nil
		}
	}
	return false, ctx.Err()
}

func isRefused(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "refused")
}

type cacheEntry struct {
	ip    string
	avail Availability
}

// Monitor keeps a cache of the latest availability per device and refreshes it
// periodically.
type Monitor struct {
	repo     device.Repository
	checker  Checker
	interval time.Duration
	log      *slog.Logger

	mu      sync.RWMutex
	cache   map[int64]cacheEntry
	trigger chan struct{}
}

func NewMonitor(repo device.Repository, checker Checker, interval time.Duration, log *slog.Logger) *Monitor {
	return &Monitor{
		repo: repo, checker: checker, interval: interval, log: log,
		cache: map[int64]cacheEntry{}, trigger: make(chan struct{}, 1),
	}
}

// Get returns the cached availability for d.
func (m *Monitor) Get(d device.Device) Availability {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.cache[d.ID]; ok && e.ip == d.IP {
		return e.avail
	}
	return Availability{Status: Unknown}
}

// Check probes d now and stores the result.
func (m *Monitor) Check(ctx context.Context, d device.Device) Availability {
	if d.IP == "" {
		return Availability{Status: Unknown}
	}
	ok, err := m.checker.Check(ctx, d.IP)
	if err != nil {
		return m.Get(d)
	}
	now := time.Now().UTC()
	a := Availability{Status: Unreachable, CheckedAt: &now}
	if ok {
		a.Status = Reachable
	}
	m.mu.Lock()
	m.cache[d.ID] = cacheEntry{ip: d.IP, avail: a}
	m.mu.Unlock()
	return a
}

func (m *Monitor) Forget(id int64) {
	m.mu.Lock()
	delete(m.cache, id)
	m.mu.Unlock()
}

// Trigger requests an immediate sweep without blocking.
func (m *Monitor) Trigger() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Run sweeps until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.interval)
	defer t.Stop()
	m.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-m.trigger:
		}
		m.sweep(ctx)
	}
}

func (m *Monitor) sweep(ctx context.Context) {
	devs, err := m.repo.List(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.log.Error("availability sweep: list devices", "error", err)
		}
		return
	}
	live := make(map[int64]bool, len(devs))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, d := range devs {
		live[d.ID] = true
		if d.IP == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			m.Check(ctx, d)
		}()
	}
	wg.Wait()
	m.mu.Lock()
	for id := range m.cache {
		if !live[id] {
			delete(m.cache, id)
		}
	}
	m.mu.Unlock()
}
