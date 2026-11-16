// Package wol implements the Wake-on-LAN protocol: MAC parsing, magic packet
// construction and UDP transmission. It deliberately knows nothing about HTTP,
// databases or the application's device model.
package wol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrInvalidMAC is returned (wrapped) for any MAC address that cannot be used.
var ErrInvalidMAC = errors.New("invalid MAC address")

// ParseMAC parses a 48-bit MAC address. It accepts the common notations
// aa:bb:cc:dd:ee:ff, aa-bb-cc-dd-ee-ff, aabb.ccdd.eeff and aabbccddeeff.
// All-zero and all-ones addresses are rejected because they can never
// identify a single machine.
func ParseMAC(s string) (net.HardwareAddr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: value is empty", ErrInvalidMAC)
	}

	var parts []string
	var wantLen, wantParts int
	switch {
	case strings.Contains(s, ":"):
		parts, wantParts, wantLen = strings.Split(s, ":"), 6, 2
	case strings.Contains(s, "-"):
		parts, wantParts, wantLen = strings.Split(s, "-"), 6, 2
	case strings.Contains(s, "."):
		parts, wantParts, wantLen = strings.Split(s, "."), 3, 4
	default:
		parts, wantParts, wantLen = []string{s}, 1, 12
	}
	if len(parts) != wantParts {
		return nil, fmt.Errorf("%w: %q is not a 6-byte address", ErrInvalidMAC, s)
	}
	for _, p := range parts {
		if len(p) != wantLen {
			return nil, fmt.Errorf("%w: %q is not a 6-byte address", ErrInvalidMAC, s)
		}
	}
	raw, err := hex.DecodeString(strings.Join(parts, ""))
	if err != nil || len(raw) != 6 {
		return nil, fmt.Errorf("%w: %q contains non-hexadecimal characters", ErrInvalidMAC, s)
	}
	if bytes.Equal(raw, make([]byte, 6)) {
		return nil, fmt.Errorf("%w: all-zero address", ErrInvalidMAC)
	}
	if bytes.Equal(raw, bytes.Repeat([]byte{0xff}, 6)) {
		return nil, fmt.Errorf("%w: broadcast address", ErrInvalidMAC)
	}
	return net.HardwareAddr(raw), nil
}

// NormalizeMAC returns the canonical lower-case colon-separated form.
func NormalizeMAC(s string) (string, error) {
	hw, err := ParseMAC(s)
	if err != nil {
		return "", err
	}
	return hw.String(), nil
}
