package wol

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestParseMAC(t *testing.T) {
	valid := map[string]string{
		"aa:bb:cc:dd:ee:ff":    "aa:bb:cc:dd:ee:ff",
		"AA-BB-CC-DD-EE-FF":    "aa:bb:cc:dd:ee:ff",
		"aabb.ccdd.eeff":       "aa:bb:cc:dd:ee:ff",
		"AABBCCDDEEFF":         "aa:bb:cc:dd:ee:ff",
		"  01:23:45:67:89:ab ": "01:23:45:67:89:ab",
	}
	for in, want := range valid {
		got, err := NormalizeMAC(in)
		if err != nil || got != want {
			t.Errorf("NormalizeMAC(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	invalid := []string{
		"", "   ", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff:00", "gg:bb:cc:dd:ee:ff",
		"aa:bb:cc:dd:ee:f", "aa:bb-cc:dd-ee:ff", "aabbccddeef", "aabbccddeeff00",
		"00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "aabb.ccdd", "aa:bb:cc:dd:ee:fff",
		"02:00:5e:10:00:00:00:01", // EUI-64
	}
	for _, in := range invalid {
		if _, err := ParseMAC(in); !errors.Is(err, ErrInvalidMAC) {
			t.Errorf("ParseMAC(%q) error = %v; want ErrInvalidMAC", in, err)
		}
	}
}

func TestMagicPacket(t *testing.T) {
	mac, _ := ParseMAC("01:23:45:67:89:ab")
	pkt, err := NewMagicPacket(mac)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt) != 102 {
		t.Fatalf("length = %d, want 102", len(pkt))
	}
	if !bytes.Equal(pkt[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Errorf("header = % x, want six 0xff bytes", pkt[:6])
	}
	for i := 0; i < 16; i++ {
		if got := pkt[6+i*6 : 12+i*6]; !bytes.Equal(got, mac) {
			t.Errorf("repetition %d = % x, want % x", i, got, []byte(mac))
		}
	}
}

func TestMagicPacketRejectsWrongLength(t *testing.T) {
	for _, mac := range []net.HardwareAddr{nil, {1, 2, 3}, make(net.HardwareAddr, 8)} {
		if _, err := NewMagicPacket(mac); !errors.Is(err, ErrInvalidMAC) {
			t.Errorf("NewMagicPacket(%v) error = %v", mac, err)
		}
	}
}

// The UDP sender is exercised against a real loopback socket: no physical
// hardware is involved.
func TestUDPSenderDeliversPacket(t *testing.T) {
	pc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skipf("cannot listen on loopback UDP: %v", err)
	}
	defer pc.Close()
	port := pc.LocalAddr().(*net.UDPAddr).Port

	mac, _ := ParseMAC("aa:bb:cc:dd:ee:ff")
	want, _ := NewMagicPacket(mac)
	err = UDPSender{Timeout: time.Second}.Send(context.Background(), want,
		Target{Broadcast: net.IPv4(127, 0, 0, 1), Port: port, SourceIP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 256)
	n, _, err := pc.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("received packet differs from sent packet")
	}
}

func TestUDPSenderValidatesTarget(t *testing.T) {
	s := UDPSender{}
	bad := []Target{
		{},
		{Broadcast: net.ParseIP("::1"), Port: 9},
		{Broadcast: net.IPv4(10, 0, 0, 255), Port: 0},
		{Broadcast: net.IPv4(10, 0, 0, 255), Port: 70000},
	}
	for _, tg := range bad {
		if err := s.Send(context.Background(), []byte{1}, tg); err == nil {
			t.Errorf("Send(%+v) succeeded, want error", tg)
		}
	}
}

func TestUDPSenderBadSourceReturnsError(t *testing.T) {
	// 192.0.2.1 (TEST-NET-1) is not assigned to any local interface.
	err := UDPSender{Timeout: time.Second}.Send(context.Background(), []byte{1},
		Target{Broadcast: net.IPv4(127, 0, 0, 1), Port: 9, SourceIP: net.IPv4(192, 0, 2, 1)})
	if err == nil {
		t.Fatal("expected error binding to a non-local source address")
	}
}

type fakeSender struct {
	pkt    []byte
	target Target
	err    error
	calls  int
}

func (f *fakeSender) Send(_ context.Context, p []byte, t Target) error {
	f.calls++
	f.pkt, f.target = p, t
	return f.err
}

func TestEngineWake(t *testing.T) {
	fs := &fakeSender{}
	e := NewEngine(fs)
	tg := Target{Broadcast: net.IPv4(10, 1, 2, 255), Port: 7}
	if err := e.Wake(context.Background(), "AA-BB-CC-DD-EE-FF", tg); err != nil {
		t.Fatal(err)
	}
	if fs.calls != 1 || len(fs.pkt) != 102 || fs.target.Port != 7 {
		t.Errorf("unexpected send: calls=%d len=%d target=%+v", fs.calls, len(fs.pkt), fs.target)
	}
}

func TestEngineWakeErrors(t *testing.T) {
	fs := &fakeSender{}
	e := NewEngine(fs)
	if err := e.Wake(context.Background(), "nope", Target{}); !errors.Is(err, ErrInvalidMAC) {
		t.Errorf("invalid MAC: err = %v", err)
	}
	if fs.calls != 0 {
		t.Error("sender must not be called for an invalid MAC")
	}

	boom := errors.New("boom")
	fs.err = boom
	if err := e.Wake(context.Background(), "aa:bb:cc:dd:ee:ff", Target{}); !errors.Is(err, boom) {
		t.Errorf("sender error not propagated: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fs.calls = 0
	if err := e.Wake(ctx, "aa:bb:cc:dd:ee:ff", Target{}); !errors.Is(err, context.Canceled) || fs.calls != 0 {
		t.Errorf("cancelled context: err=%v calls=%d", err, fs.calls)
	}
}
