package wol

import (
	"context"
	"fmt"
)

// Engine validates a MAC, builds the magic packet and hands it to a Sender.
// Packet generation and transmission stay separate: see NewMagicPacket and Sender.
type Engine struct {
	sender Sender
}

func NewEngine(sender Sender) *Engine { return &Engine{sender: sender} }

// Wake sends a magic packet for mac to target. A nil error means the packet was
// handed to the network stack; it does NOT prove the machine woke up.
func (e *Engine) Wake(ctx context.Context, mac string, t Target) error {
	hw, err := ParseMAC(mac)
	if err != nil {
		return err
	}
	pkt, err := NewMagicPacket(hw)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("wake cancelled: %w", err)
	}
	return e.sender.Send(ctx, pkt, t)
}
