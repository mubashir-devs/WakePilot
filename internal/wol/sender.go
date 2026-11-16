package wol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultPort is the conventional Wake-on-LAN UDP port ("discard").
const DefaultPort = 9

// Target describes where a packet is sent.
type Target struct {
	// Broadcast is the destination IPv4 address (usually a subnet broadcast).
	Broadcast net.IP
	// Port is the destination UDP port.
	Port int
	// SourceIP, when set, binds the socket to a local address so the packet
	// leaves through the interface that owns it.
	SourceIP net.IP
}

// Sender transmits a raw packet. It is an interface so callers can substitute
// a fake in tests.
type Sender interface {
	Send(ctx context.Context, packet []byte, t Target) error
}

// UDPSender sends packets over UDP broadcast.
//
// Go's net package enables SO_BROADCAST on every UDP socket it creates, so no
// platform-specific socket option handling is required here.
type UDPSender struct {
	// Timeout bounds the whole send. Zero means 5 seconds.
	Timeout time.Duration
}

func (s UDPSender) Send(ctx context.Context, packet []byte, t Target) error {
	if err := t.validate(); err != nil {
		return err
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d := net.Dialer{}
	if t.SourceIP != nil {
		d.LocalAddr = &net.UDPAddr{IP: t.SourceIP}
	}
	dst := &net.UDPAddr{IP: t.Broadcast, Port: t.Port}
	conn, err := d.DialContext(ctx, "udp4", dst.String())
	if err != nil {
		return fmt.Errorf("open UDP socket to %s: %w", dst, err)
	}
	defer conn.Close()

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(dl)
	}
	n, err := conn.Write(packet)
	if err != nil {
		return fmt.Errorf("send magic packet to %s: %w", dst, err)
	}
	if n != len(packet) {
		return fmt.Errorf("send magic packet to %s: short write (%d of %d bytes)", dst, n, len(packet))
	}
	return ctx.Err()
}

func (t Target) validate() error {
	if t.Broadcast == nil || t.Broadcast.To4() == nil {
		return errors.New("target broadcast address must be a valid IPv4 address")
	}
	if t.Port < 1 || t.Port > 65535 {
		return fmt.Errorf("target port %d out of range 1-65535", t.Port)
	}
	return nil
}
