package network

import (
	"errors"
	"net"
	"testing"
)

type fakeLister []Interface

func (f fakeLister) Interfaces() ([]Interface, error) { return f, nil }

func addr(cidr string) Address {
	ip, n, _ := net.ParseCIDR(cidr)
	return NewAddress(ip, n.Mask)
}

func iface(name string, up bool, cidrs ...string) Interface {
	i := Interface{Name: name, Up: up, SupportsBroadcast: true}
	for _, c := range cidrs {
		i.Addresses = append(i.Addresses, addr(c))
	}
	return i
}

func TestNewAddress(t *testing.T) {
	tests := []struct {
		cidr, network, broadcast, mask string
	}{
		{"192.168.1.20/24", "192.168.1.0", "192.168.1.255", "255.255.255.0"},
		{"10.20.30.40/16", "10.20.0.0", "10.20.255.255", "255.255.0.0"},
		{"172.16.5.9/20", "172.16.0.0", "172.16.15.255", "255.255.240.0"},
		{"10.0.0.5/32", "10.0.0.5", "", "255.255.255.255"},
		{"10.0.0.4/31", "10.0.0.4", "", "255.255.255.254"},
		{"10.0.0.5/30", "10.0.0.4", "10.0.0.7", "255.255.255.252"},
	}
	for _, tc := range tests {
		a := addr(tc.cidr)
		if a.Network != tc.network || a.Broadcast != tc.broadcast || a.Netmask != tc.mask {
			t.Errorf("%s: got network=%s broadcast=%q mask=%s", tc.cidr, a.Network, a.Broadcast, a.Netmask)
		}
	}
}

func TestResolve(t *testing.T) {
	eth := iface("eth0", true, "192.168.1.10/24")
	wlan := iface("wlan0", true, "10.0.5.7/16")
	multi := iface("br0", true, "172.16.0.1/24", "172.16.9.1/24")
	down := iface("eth1", false, "192.168.77.2/24")
	lo := Interface{Name: "lo", Up: true, Loopback: true, Addresses: []Address{addr("127.0.0.1/8")}}
	all := fakeLister{eth, wlan, multi, down, lo}

	tests := []struct {
		name       string
		defaultIf  string
		req        ResolveRequest
		wantBcast  string
		wantIface  string
		wantSource string
		wantErr    error
		wantUp     *bool
	}{
		{name: "by interface", req: ResolveRequest{Interface: "eth0"}, wantBcast: "192.168.1.255", wantIface: "eth0", wantSource: "192.168.1.10"},
		{name: "non-/24 subnet is not assumed", req: ResolveRequest{Interface: "wlan0"}, wantBcast: "10.0.255.255", wantIface: "wlan0", wantSource: "10.0.5.7"},
		{name: "by device IP", req: ResolveRequest{DeviceIP: "10.0.9.9"}, wantBcast: "10.0.255.255", wantIface: "wlan0", wantSource: "10.0.5.7"},
		{name: "explicit directed broadcast", req: ResolveRequest{Broadcast: "192.168.1.255"}, wantBcast: "192.168.1.255", wantIface: "eth0", wantSource: "192.168.1.10"},
		{name: "limited broadcast needs no interface", req: ResolveRequest{Broadcast: "255.255.255.255"}, wantBcast: "255.255.255.255"},
		{name: "limited broadcast on interface binds source", req: ResolveRequest{Broadcast: "255.255.255.255", Interface: "wlan0"}, wantBcast: "255.255.255.255", wantIface: "wlan0", wantSource: "10.0.5.7"},
		{name: "default interface used", defaultIf: "eth0", req: ResolveRequest{}, wantBcast: "192.168.1.255", wantIface: "eth0", wantSource: "192.168.1.10"},
		{name: "device interface beats default", defaultIf: "eth0", req: ResolveRequest{Interface: "wlan0"}, wantBcast: "10.0.255.255", wantIface: "wlan0", wantSource: "10.0.5.7"},
		{name: "multi-subnet interface picked by device IP", req: ResolveRequest{Interface: "br0", DeviceIP: "172.16.9.50"}, wantBcast: "172.16.9.255", wantIface: "br0", wantSource: "172.16.9.1"},
		{name: "multi-subnet interface without match is ambiguous", req: ResolveRequest{Interface: "br0"}, wantErr: ErrAmbiguous},
		{name: "unknown interface", req: ResolveRequest{Interface: "nope"}, wantErr: ErrInterfaceNotFound},
		{name: "unknown default interface", defaultIf: "nope", req: ResolveRequest{}, wantErr: ErrInterfaceNotFound},
		{name: "arbitrary remote broadcast refused", req: ResolveRequest{Broadcast: "8.8.8.255"}, wantErr: ErrBroadcastNotAllowed},
		{name: "unicast address refused as broadcast", req: ResolveRequest{Broadcast: "192.168.1.50"}, wantErr: ErrBroadcastNotAllowed},
		{name: "broadcast of another interface refused", req: ResolveRequest{Broadcast: "10.0.255.255", Interface: "eth0"}, wantErr: ErrBroadcastNotAllowed},
		{name: "loopback broadcast refused without naming it", req: ResolveRequest{Broadcast: "127.255.255.255"}, wantErr: ErrBroadcastNotAllowed},
		{name: "IP in no local subnet", req: ResolveRequest{DeviceIP: "203.0.113.5"}, wantErr: ErrNoRoute},
		{name: "nothing to go on", req: ResolveRequest{}, wantErr: ErrNoRoute},
		{name: "bad broadcast", req: ResolveRequest{Broadcast: "xyz"}, wantErr: ErrInvalidAddress},
		{name: "bad device IP", req: ResolveRequest{DeviceIP: "1.2.3"}, wantErr: ErrInvalidAddress},
		{name: "reports down interface", req: ResolveRequest{Interface: "eth1"}, wantBcast: "192.168.77.255", wantIface: "eth1", wantSource: "192.168.77.2", wantUp: ptr(false)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewService(all, tc.defaultIf).Resolve(tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || !IsConfigError(err) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Broadcast.String() != tc.wantBcast || got.Interface != tc.wantIface {
				t.Errorf("got broadcast=%s iface=%q; want %s %q", got.Broadcast, got.Interface, tc.wantBcast, tc.wantIface)
			}
			if tc.wantSource != "" && got.SourceIP.String() != tc.wantSource {
				t.Errorf("source = %s, want %s", got.SourceIP, tc.wantSource)
			}
			if tc.wantSource == "" && got.SourceIP != nil {
				t.Errorf("source = %s, want none", got.SourceIP)
			}
			if tc.wantUp != nil && got.Up != *tc.wantUp {
				t.Errorf("up = %v", got.Up)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestResolveAmbiguousDeviceIP(t *testing.T) {
	// Two interfaces on overlapping subnets: refuse to guess.
	l := fakeLister{iface("a", true, "10.0.0.1/16"), iface("b", true, "10.0.1.1/24")}
	_, err := NewService(l, "").Resolve(ResolveRequest{DeviceIP: "10.0.1.50"})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
}

func TestResolveNoBroadcastSubnet(t *testing.T) {
	l := fakeLister{iface("ppp0", true, "10.9.9.2/32")}
	if _, err := NewService(l, "").Resolve(ResolveRequest{Interface: "ppp0"}); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

func TestSystemListerDoesNotFail(t *testing.T) {
	if _, err := (SystemLister{}).Interfaces(); err != nil {
		t.Fatal(err)
	}
}
