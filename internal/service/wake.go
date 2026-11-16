package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wakepilot/internal/device"
	"wakepilot/internal/network"
	"wakepilot/internal/wol"
)

// Waker sends a magic packet. *wol.Engine implements it.
type Waker interface {
	Wake(ctx context.Context, mac string, t wol.Target) error
}

// Sentinel errors for the two failure classes API callers distinguish.
var (
	ErrWakeConfig = errors.New("device network configuration cannot be used")
	ErrWakeSend   = errors.New("failed to send magic packet")
)

type WakeOptions struct {
	SendTimeout    time.Duration
	VerifyEnabled  bool
	VerifyTimeout  time.Duration
	VerifyInterval time.Duration
}

type WakeService struct {
	devices device.Repository
	history device.HistoryRepository
	net     NetworkResolver
	waker   Waker
	monitor *Monitor
	opts    WakeOptions
	log     *slog.Logger

	bgCtx  context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWakeService(devs device.Repository, hist device.HistoryRepository, n NetworkResolver, w Waker, m *Monitor, opts WakeOptions, log *slog.Logger) *WakeService {
	ctx, cancel := context.WithCancel(context.Background())
	return &WakeService{devices: devs, history: hist, net: n, waker: w, monitor: m, opts: opts, log: log, bgCtx: ctx, cancel: cancel}
}

// Close stops background verification and waits for it to finish.
func (s *WakeService) Close() {
	s.cancel()
	s.wg.Wait()
}

// Wake sends a magic packet to the device and records the attempt. The returned
// record is populated (with ID) whenever the attempt was recorded, even if the
// returned error is non-nil. "sent" means the packet left this machine only.
func (s *WakeService) Wake(ctx context.Context, deviceID int64) (device.WakeRecord, error) {
	d, err := s.devices.Get(ctx, deviceID)
	if err != nil {
		return device.WakeRecord{}, err
	}
	rec := device.WakeRecord{
		DeviceID: &d.ID, DeviceName: d.Name, MAC: d.MAC, Port: d.Port, Interface: d.Interface,
		Reachability: device.ReachNotChecked, RequestedAt: time.Now(),
	}

	fail := func(kind error, cause error) (device.WakeRecord, error) {
		rec.Status = device.StatusFailed
		rec.Error = cause.Error()
		saved, serr := s.history.Add(context.WithoutCancel(ctx), rec)
		if serr != nil {
			s.log.Error("record wake failure", "device", d.ID, "error", serr)
			saved = rec
		}
		s.log.Warn("wake failed", "device", d.ID, "name", d.Name, "error", cause)
		return saved, fmt.Errorf("%w: %w", kind, cause)
	}

	res, err := s.net.Resolve(network.ResolveRequest{Interface: d.Interface, Broadcast: d.Broadcast, DeviceIP: d.IP})
	if err != nil {
		if network.IsConfigError(err) {
			return fail(ErrWakeConfig, err)
		}
		return fail(ErrWakeSend, err)
	}
	rec.Broadcast, rec.Interface = res.Broadcast.String(), res.Interface
	if !res.Up {
		return fail(ErrWakeConfig, fmt.Errorf("network interface %s is down", res.Interface))
	}

	sendCtx, cancel := context.WithTimeout(ctx, s.opts.SendTimeout)
	defer cancel()
	if err := s.waker.Wake(sendCtx, d.MAC, wol.Target{Broadcast: res.Broadcast, Port: d.Port, SourceIP: res.SourceIP}); err != nil {
		return fail(ErrWakeSend, err)
	}

	rec.Status = device.StatusSent
	if s.opts.VerifyEnabled && d.IP != "" {
		rec.Reachability = device.ReachPending
	}
	saved, err := s.history.Add(context.WithoutCancel(ctx), rec)
	if err != nil {
		return rec, fmt.Errorf("packet sent but recording failed: %w", err)
	}
	s.log.Info("magic packet sent", "device", d.ID, "name", d.Name, "broadcast", saved.Broadcast, "port", d.Port, "interface", saved.Interface)

	if saved.Reachability == device.ReachPending {
		s.wg.Add(1)
		go s.verify(saved.ID, d)
	}
	return saved, nil
}

// verify polls the device until it answers or VerifyTimeout elapses, then
// records the result. Silence is recorded as "unknown", never "off".
func (s *WakeService) verify(recordID int64, d device.Device) {
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(s.bgCtx, s.opts.VerifyTimeout)
	defer cancel()
	result := device.ReachUnknown
	t := time.NewTicker(s.opts.VerifyInterval)
	defer t.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-t.C:
			if s.monitor.Check(ctx, d).Status == Reachable {
				result = device.ReachReachable
				break loop
			}
		}
	}
	// Shutdown may have cancelled us; still try to persist a final state.
	if err := s.history.UpdateReachability(context.WithoutCancel(s.bgCtx), recordID, result, time.Now()); err != nil {
		s.log.Error("record reachability", "record", recordID, "error", err)
	}
}

// History lists wake records, newest first.
func (s *WakeService) History(ctx context.Context, f device.HistoryFilter) ([]device.WakeRecord, int, error) {
	return s.history.List(ctx, f)
}

// Overview summarises the dashboard.
type Overview struct {
	TotalDevices   int                 `json:"totalDevices"`
	Reachable      int                 `json:"reachableDevices"`
	NoResponse     int                 `json:"noResponseDevices"` // unreachable or unknown
	TotalWakes     int                 `json:"totalWakeRequests"`
	RecentActivity []device.WakeRecord `json:"recentActivity"`
}

func (s *WakeService) Overview(ctx context.Context) (Overview, error) {
	devs, err := s.devices.List(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{TotalDevices: len(devs)}
	for _, d := range devs {
		if s.monitor.Get(d).Status == Reachable {
			o.Reachable++
		} else {
			o.NoResponse++
		}
	}
	o.RecentActivity, o.TotalWakes, err = s.history.List(ctx, device.HistoryFilter{Limit: 8})
	return o, err
}
