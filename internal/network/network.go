// Package network discovers local interfaces and resolves which interface and
// broadcast address a wake packet should use. It never guesses: ambiguous or
// unknown configurations produce explicit errors.
package network

import (
	"errors"
	"fmt"
	"net"
	"sort"
)

var (
	ErrInterfaceNotFound   = errors.New("network interface not found")
	ErrBroadcastNotAllowed = errors.New("broadcast address not allowed")
	ErrAmbiguous           = errors.New("ambiguous network configuration")
	ErrNoRoute             = errors.New("no matching network interface")
	ErrInvalidAddress      = errors.New("invalid IPv4 address")
)

// IsConfigError reports whether err describes a user-correctable configuration problem.
func IsConfigError(err error) bool {
	for _, target := range []error{ErrInterfaceNotFound, ErrBroadcastNotAllowed, ErrAmbiguous, ErrNoRoute, ErrInvalidAddress} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// Address is one IPv4 address bound to an interface.
type Address struct {
	IP        string `json:"ip"`
	CIDR      string `json:"cidr"`
	Netmask   string `json:"netmask"`
	Network   string `json:"network"`
	Broadcast string `json:"broadcast"` // empty for /31 and /32
	PrefixLen int    `json:"prefixLen"`
}

// Interface is a local network interface with its IPv4 addresses.
type Interface struct {
	Name              string    `json:"name"`
	Index             int       `json:"index"`
	Up                bool      `json:"up"`
	Loopback          bool      `json:"loopback"`
	SupportsBroadcast bool      `json:"supportsBroadcast"`
	Addresses         []Address `json:"addresses"`
}

// Lister enumerates interfaces. Replace it in tests.
type Lister interface {
	Interfaces() ([]Interface, error)
}

// SystemLister reads interfaces from the operating system.
type SystemLister struct{}

func (SystemLister) Interfaces() ([]Interface, error) {
	sys, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	var out []Interface
	for _, ni := range sys {
		addrs, err := ni.Addrs()
		if err != nil {
			continue
		}
		iface := Interface{
			Name:              ni.Name,
			Index:             ni.Index,
			Up:                ni.Flags&net.FlagUp != 0,
			Loopback:          ni.Flags&net.FlagLoopback != 0,
			SupportsBroadcast: ni.Flags&net.FlagBroadcast != 0,
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue // IPv4 only for now
			}
			iface.Addresses = append(iface.Addresses, NewAddress(ipn.IP, ipn.Mask))
		}
		if len(iface.Addresses) > 0 {
			out = append(out, iface)
		}
	}
	return out, nil
}

// NewAddress derives subnet details from an IPv4 address and mask.
func NewAddress(ip net.IP, mask net.IPMask) Address {
	ip4 := ip.To4()
	ones, bits := mask.Size()
	if len(mask) == net.IPv6len {
		mask = mask[12:]
	}
	network := ip4.Mask(mask)
	a := Address{
		IP:        ip4.String(),
		CIDR:      fmt.Sprintf("%s/%d", ip4, ones),
		Netmask:   net.IP(mask).String(),
		Network:   network.String(),
		PrefixLen: ones,
	}
	if bits == 32 && ones <= 30 {
		a.Broadcast = BroadcastOf(network, mask).String()
	}
	return a
}

// BroadcastOf returns the directed broadcast address of a network.
func BroadcastOf(network net.IP, mask net.IPMask) net.IP {
	n := network.To4()
	b := make(net.IP, 4)
	for i := range b {
		b[i] = n[i] | ^mask[len(mask)-4+i]
	}
	return b
}

func (a Address) contains(ip net.IP) bool {
	_, n, err := net.ParseCIDR(a.CIDR)
	if err != nil {
		return false
	}
	// ParseCIDR on a host CIDR masks to the network; contains works on that.
	return n.Contains(ip)
}

// LimitedBroadcast is 255.255.255.255.
var LimitedBroadcast = net.IPv4bcast.To4()

// ResolveRequest carries what is known about a device.
type ResolveRequest struct {
	Interface string // optional interface name
	Broadcast string // optional explicit broadcast address
	DeviceIP  string // optional device IPv4 address
}

// Resolved is the outcome: where to send and through which interface.
type Resolved struct {
	Broadcast net.IP
	SourceIP  net.IP // nil when not bound to a specific interface address
	Interface string // interface name, empty if unknown (limited broadcast without interface)
	Up        bool   // false if the chosen interface is down
}

// Service resolves wake destinations from the local interface list.
type Service struct {
	lister           Lister
	defaultInterface string
}

func NewService(l Lister, defaultInterface string) *Service {
	return &Service{lister: l, defaultInterface: defaultInterface}
}

func (s *Service) DefaultInterface() string { return s.defaultInterface }

// List returns interfaces sorted by name.
func (s *Service) List() ([]Interface, error) {
	ifs, err := s.lister.Interfaces()
	if err != nil {
		return nil, err
	}
	sort.Slice(ifs, func(i, j int) bool { return ifs[i].Name < ifs[j].Name })
	return ifs, nil
}

// Resolve picks the destination for req using this precedence:
//  1. explicit broadcast (must belong to a local subnet or be 255.255.255.255)
//  2. named interface, else the configured default interface
//  3. the single interface whose subnet contains the device IP
//
// Anything ambiguous or unmatched is an error; nothing is guessed.
func (s *Service) Resolve(req ResolveRequest) (Resolved, error) {
	ifaces, err := s.List()
	if err != nil {
		return Resolved{}, err
	}

	var devIP net.IP
	if req.DeviceIP != "" {
		if devIP = parseIPv4(req.DeviceIP); devIP == nil {
			return Resolved{}, fmt.Errorf("%w: device IP %q", ErrInvalidAddress, req.DeviceIP)
		}
	}

	var named *Interface
	if req.Interface != "" {
		named = findInterface(ifaces, req.Interface)
		if named == nil {
			return Resolved{}, fmt.Errorf("%w: %q", ErrInterfaceNotFound, req.Interface)
		}
	}

	if req.Broadcast != "" {
		return s.resolveExplicit(ifaces, named, req.Broadcast, devIP)
	}

	if named == nil && s.defaultInterface != "" {
		named = findInterface(ifaces, s.defaultInterface)
		if named == nil {
			return Resolved{}, fmt.Errorf("%w: default interface %q is not present", ErrInterfaceNotFound, s.defaultInterface)
		}
	}
	if named != nil {
		addr, err := pickAddress(*named, devIP)
		if err != nil {
			return Resolved{}, err
		}
		return resolvedFrom(*named, addr)
	}

	if devIP == nil {
		return Resolved{}, fmt.Errorf("%w: set an interface, a broadcast address or a device IP", ErrNoRoute)
	}
	type match struct {
		iface Interface
		addr  Address
	}
	var matches []match
	for _, i := range ifaces {
		if i.Loopback {
			continue
		}
		for _, a := range i.Addresses {
			if a.Broadcast != "" && a.contains(devIP) {
				matches = append(matches, match{i, a})
			}
		}
	}
	switch len(matches) {
	case 0:
		return Resolved{}, fmt.Errorf("%w: no local interface is in the same subnet as %s; choose an interface or broadcast address explicitly", ErrNoRoute, devIP)
	case 1:
		return resolvedFrom(matches[0].iface, matches[0].addr)
	default:
		return Resolved{}, fmt.Errorf("%w: %s matches %d interfaces; choose one explicitly", ErrAmbiguous, devIP, len(matches))
	}
}

func (s *Service) resolveExplicit(ifaces []Interface, named *Interface, raw string, devIP net.IP) (Resolved, error) {
	b := parseIPv4(raw)
	if b == nil {
		return Resolved{}, fmt.Errorf("%w: broadcast %q", ErrInvalidAddress, raw)
	}
	if b.Equal(LimitedBroadcast) {
		r := Resolved{Broadcast: LimitedBroadcast, Up: true}
		if named != nil {
			addr, err := pickAddress(*named, devIP)
			if err != nil {
				return Resolved{}, err
			}
			r.Interface, r.SourceIP, r.Up = named.Name, net.ParseIP(addr.IP).To4(), named.Up
		}
		return r, nil
	}
	candidates := ifaces
	if named != nil {
		candidates = []Interface{*named}
	}
	for _, i := range candidates {
		if i.Loopback && named == nil {
			continue
		}
		for _, a := range i.Addresses {
			if a.Broadcast == b.String() {
				return Resolved{Broadcast: b, SourceIP: net.ParseIP(a.IP).To4(), Interface: i.Name, Up: i.Up}, nil
			}
		}
	}
	if named != nil {
		return Resolved{}, fmt.Errorf("%w: %s is not a broadcast address of interface %s", ErrBroadcastNotAllowed, b, named.Name)
	}
	return Resolved{}, fmt.Errorf("%w: %s is not the broadcast address of any local subnet (or 255.255.255.255)", ErrBroadcastNotAllowed, b)
}

func resolvedFrom(i Interface, a Address) (Resolved, error) {
	if a.Broadcast == "" {
		return Resolved{}, fmt.Errorf("%w: %s on %s has no broadcast address (/%d)", ErrNoRoute, a.IP, i.Name, a.PrefixLen)
	}
	return Resolved{
		Broadcast: net.ParseIP(a.Broadcast).To4(),
		SourceIP:  net.ParseIP(a.IP).To4(),
		Interface: i.Name,
		Up:        i.Up,
	}, nil
}

// pickAddress chooses an address on iface: the one containing devIP, or the
// only one. Multiple candidates without a match is ambiguous.
func pickAddress(i Interface, devIP net.IP) (Address, error) {
	var usable []Address
	for _, a := range i.Addresses {
		if a.Broadcast != "" {
			usable = append(usable, a)
		}
	}
	if devIP != nil {
		for _, a := range usable {
			if a.contains(devIP) {
				return a, nil
			}
		}
	}
	switch len(usable) {
	case 0:
		return Address{}, fmt.Errorf("%w: interface %s has no IPv4 subnet with a broadcast address", ErrNoRoute, i.Name)
	case 1:
		return usable[0], nil
	default:
		return Address{}, fmt.Errorf("%w: interface %s has %d IPv4 subnets and none contains the device IP; set the broadcast address explicitly", ErrAmbiguous, i.Name, len(usable))
	}
}

func findInterface(ifaces []Interface, name string) *Interface {
	for i := range ifaces {
		if ifaces[i].Name == name {
			return &ifaces[i]
		}
	}
	return nil
}

func parseIPv4(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil
	}
	return ip.To4()
}
