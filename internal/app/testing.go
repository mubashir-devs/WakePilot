package app

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"wakepilot/internal/network"
	"wakepilot/internal/wol"
)

// QuietLogger discards all log output.
func QuietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// FakeSender records packets instead of transmitting them. Safe for concurrent use.
type FakeSender struct {
	mu   sync.Mutex
	Sent []SentPacket
	Err  error
}

type SentPacket struct {
	Packet []byte
	Target wol.Target
}

func (f *FakeSender) Send(_ context.Context, p []byte, t wol.Target) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, SentPacket{Packet: append([]byte(nil), p...), Target: t})
	return nil
}

func (f *FakeSender) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Sent)
}

// FakeLister returns a fixed interface list.
type FakeLister []network.Interface

func (l FakeLister) Interfaces() ([]network.Interface, error) { return l, nil }

// FakeChecker answers availability probes from a settable value.
type FakeChecker struct {
	mu    sync.Mutex
	Alive bool
}

func (c *FakeChecker) Check(_ context.Context, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Alive, nil
}

func (c *FakeChecker) SetAlive(v bool) {
	c.mu.Lock()
	c.Alive = v
	c.mu.Unlock()
}
