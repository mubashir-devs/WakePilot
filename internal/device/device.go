// Package device holds the device and wake-history domain types, input
// validation, and the repository interfaces implemented by storage.
package device

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"time"

	"wakepilot/internal/wol"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicateMAC = errors.New("a device with this MAC address already exists")
)

type Device struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	MAC         string    `json:"mac"`
	IP          string    `json:"ip"`
	Broadcast   string    `json:"broadcast"`
	Port        int       `json:"port"`
	Interface   string    `json:"interface"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Input is what clients submit to create or replace a device.
type Input struct {
	Name        string `json:"name"`
	MAC         string `json:"mac"`
	IP          string `json:"ip"`
	Broadcast   string `json:"broadcast"`
	Port        int    `json:"port"`
	Interface   string `json:"interface"`
	Description string `json:"description"`
}

// ValidationError maps field names to messages.
type ValidationError map[string]string

func (v ValidationError) Error() string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+v[k])
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Normalize trims and canonicalises the input in place and validates every field.
// A zero Port is replaced with defaultPort.
func (in *Input) Normalize(defaultPort int) error {
	errs := ValidationError{}
	in.Name = strings.TrimSpace(in.Name)
	in.IP = strings.TrimSpace(in.IP)
	in.Broadcast = strings.TrimSpace(in.Broadcast)
	in.Interface = strings.TrimSpace(in.Interface)
	in.Description = strings.TrimSpace(in.Description)

	switch {
	case in.Name == "":
		errs["name"] = "name is required"
	case len([]rune(in.Name)) > 100:
		errs["name"] = "name must be at most 100 characters"
	}

	if mac, err := wol.NormalizeMAC(in.MAC); err != nil {
		errs["mac"] = "enter a valid 6-byte MAC address, e.g. aa:bb:cc:dd:ee:ff"
	} else {
		in.MAC = mac
	}

	if in.IP != "" && !isIPv4(in.IP) {
		errs["ip"] = "must be a valid IPv4 address"
	}
	if in.Broadcast != "" && !isIPv4(in.Broadcast) {
		errs["broadcast"] = "must be a valid IPv4 address"
	}
	if in.Port == 0 {
		in.Port = defaultPort
	}
	if in.Port < 1 || in.Port > 65535 {
		errs["port"] = "must be between 1 and 65535"
	}
	if len(in.Interface) > 64 {
		errs["interface"] = "interface name is too long"
	}
	if len([]rune(in.Description)) > 500 {
		errs["description"] = "description must be at most 500 characters"
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && !strings.Contains(s, ":")
}

// Repository persists devices.
type Repository interface {
	Create(ctx context.Context, d Device) (Device, error)
	Get(ctx context.Context, id int64) (Device, error)
	List(ctx context.Context) ([]Device, error)
	Update(ctx context.Context, d Device) (Device, error)
	Delete(ctx context.Context, id int64) error
}

// Wake record statuses and reachability values.
const (
	StatusSent   = "sent"   // packet handed to the network stack
	StatusFailed = "failed" // packet could not be sent

	ReachPending    = "pending"     // verification in progress
	ReachReachable  = "reachable"   // target answered a probe after the wake
	ReachUnknown    = "unknown"     // no answer; the device may or may not be on
	ReachNotChecked = "not_checked" // no IP configured or verification disabled
)

// WakeRecord is one wake attempt. Name/MAC are snapshots so history survives
// device deletion.
type WakeRecord struct {
	ID                    int64      `json:"id"`
	DeviceID              *int64     `json:"deviceId"`
	DeviceName            string     `json:"deviceName"`
	MAC                   string     `json:"mac"`
	Broadcast             string     `json:"broadcast"`
	Port                  int        `json:"port"`
	Interface             string     `json:"interface"`
	Status                string     `json:"status"`
	Error                 string     `json:"error"`
	Reachability          string     `json:"reachability"`
	ReachabilityCheckedAt *time.Time `json:"reachabilityCheckedAt"`
	RequestedAt           time.Time  `json:"requestedAt"`
}

type HistoryFilter struct {
	DeviceID *int64
	Limit    int
	Offset   int
}

// HistoryRepository persists wake records.
type HistoryRepository interface {
	Add(ctx context.Context, r WakeRecord) (WakeRecord, error)
	UpdateReachability(ctx context.Context, id int64, reachability string, at time.Time) error
	// ExpirePending turns records stuck in "pending" (e.g. after a restart) into "unknown".
	ExpirePending(ctx context.Context) error
	List(ctx context.Context, f HistoryFilter) ([]WakeRecord, int, error)
}
