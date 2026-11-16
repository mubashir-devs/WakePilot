package wol

import (
	"bytes"
	"fmt"
	"net"
)

// PacketLen is the size of a standard magic packet: 6 sync bytes plus 16
// repetitions of the 6-byte MAC address.
const PacketLen = 6 + 16*6

// NewMagicPacket builds the 102-byte magic packet for mac. It performs no I/O.
func NewMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("%w: expected 6 bytes, got %d", ErrInvalidMAC, len(mac))
	}
	pkt := make([]byte, 0, PacketLen)
	pkt = append(pkt, bytes.Repeat([]byte{0xff}, 6)...)
	for i := 0; i < 16; i++ {
		pkt = append(pkt, mac...)
	}
	return pkt, nil
}
