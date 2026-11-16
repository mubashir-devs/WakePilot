// Package tests exercises the whole backend (HTTP → services → SQLite → WoL
// engine) with fake networking, so no physical machines are required.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"wakepilot/internal/app"
	"wakepilot/internal/config"
	"wakepilot/internal/network"
)

type harness struct {
	t       *testing.T
	srv     *httptest.Server
	sender  *app.FakeSender
	checker *app.FakeChecker
	token   string
}

func eth0() network.Interface {
	return network.Interface{Name: "eth0", Up: true, SupportsBroadcast: true, Addresses: []network.Address{
		{IP: "192.168.1.10", CIDR: "192.168.1.10/24", Netmask: "255.255.255.0", Network: "192.168.1.0", Broadcast: "192.168.1.255", PrefixLen: 24},
	}}
}

func wlan0() network.Interface {
	return network.Interface{Name: "wlan0", Up: true, SupportsBroadcast: true, Addresses: []network.Address{
		{IP: "10.0.5.7", CIDR: "10.0.5.7/16", Netmask: "255.255.0.0", Network: "10.0.0.0", Broadcast: "10.0.255.255", PrefixLen: 16},
	}}
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	t.Helper()
	cfg := config.Default()
	cfg.Database.Path = ":memory:"
	cfg.Monitor.Enabled = false
	cfg.WoL.VerifyInterval = 10 * time.Millisecond
	cfg.WoL.VerifyTimeout = 400 * time.Millisecond
	if mutate != nil {
		mutate(&cfg)
	}
	h := &harness{t: t, sender: &app.FakeSender{}, checker: &app.FakeChecker{}}
	a, err := app.New(context.Background(), cfg, app.QuietLogger(), app.Options{
		Sender: h.sender, Checker: h.checker, Lister: app.FakeLister{eth0(), wlan0()},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	h.srv = httptest.NewServer(a.Handler)
	t.Cleanup(h.srv.Close)
	if cfg.Auth.Enabled {
		h.token = cfg.Auth.Token
	}
	return h
}

type response struct {
	Status int
	Header http.Header
	Body   struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
		Meta map[string]int `json:"meta"`
	}
}

func (h *harness) do(method, path string, body any, hdr map[string]string) response {
	h.t.Helper()
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = bytes.NewBufferString(b)
	default:
		raw, _ := json.Marshal(b)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	out := response{Status: res.StatusCode, Header: res.Header}
	raw, _ := io.ReadAll(res.Body)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.Body); err != nil {
			h.t.Fatalf("%s %s: non-JSON body %q", method, path, raw)
		}
	}
	return out
}

func (h *harness) createDevice(body map[string]any) (int64, response) {
	h.t.Helper()
	r := h.do("POST", "/api/v1/devices", body, nil)
	var d struct{ ID int64 }
	if r.Status == http.StatusCreated {
		_ = json.Unmarshal(r.Body.Data, &d)
	}
	return d.ID, r
}

func (h *harness) mustCreate(body map[string]any) int64 {
	h.t.Helper()
	id, r := h.createDevice(body)
	if r.Status != http.StatusCreated || id == 0 {
		h.t.Fatalf("create: status %d error %+v", r.Status, r.Body.Error)
	}
	return id
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

type historyRec struct {
	ID           int64  `json:"id"`
	DeviceName   string `json:"deviceName"`
	MAC          string `json:"mac"`
	Broadcast    string `json:"broadcast"`
	Port         int    `json:"port"`
	Interface    string `json:"interface"`
	Status       string `json:"status"`
	Error        string `json:"error"`
	Reachability string `json:"reachability"`
}

func (h *harness) history() []historyRec {
	h.t.Helper()
	r := h.do("GET", "/api/v1/wake-history", nil, nil)
	var recs []historyRec
	if err := json.Unmarshal(r.Body.Data, &recs); err != nil {
		h.t.Fatal(err)
	}
	return recs
}

var nas = map[string]any{"name": "NAS", "mac": "AA-BB-CC-DD-EE-01", "ip": "192.168.1.50", "interface": "eth0", "description": "Basement"}

func TestHealthIsOpenAndMinimal(t *testing.T) {
	h := newHarness(t, nil)
	r := h.do("GET", "/api/v1/health", nil, nil)
	if r.Status != 200 {
		t.Fatalf("status %d", r.Status)
	}
	var d map[string]any
	_ = json.Unmarshal(r.Body.Data, &d)
	if d["status"] != "ok" || d["authRequired"] != false {
		t.Errorf("health = %v", d)
	}
}

func TestDeviceCRUD(t *testing.T) {
	h := newHarness(t, nil)

	id, r := h.createDevice(nas)
	if r.Status != 201 || r.Header.Get("Location") != "/api/v1/devices/"+strconv.FormatInt(id, 10) {
		t.Fatalf("create: %d %v", r.Status, r.Header)
	}
	var created map[string]any
	_ = json.Unmarshal(r.Body.Data, &created)
	if created["mac"] != "aa:bb:cc:dd:ee:01" || created["port"] != float64(9) {
		t.Errorf("normalisation/default port: %v", created)
	}
	if av := created["availability"].(map[string]any); av["status"] != "unknown" {
		t.Errorf("fresh device availability = %v", av)
	}

	path := "/api/v1/devices/" + strconv.FormatInt(id, 10)
	if r := h.do("GET", path, nil, nil); r.Status != 200 {
		t.Errorf("get: %d", r.Status)
	}

	upd := map[string]any{"name": "NAS 2", "mac": "aa:bb:cc:dd:ee:01", "ip": "192.168.1.50", "interface": "eth0", "port": 7}
	r = h.do("PUT", path, upd, nil)
	var got map[string]any
	_ = json.Unmarshal(r.Body.Data, &got)
	if r.Status != 200 || got["name"] != "NAS 2" || got["port"] != float64(7) || got["description"] != "" {
		t.Errorf("put: %d %v", r.Status, got)
	}

	r = h.do("GET", "/api/v1/devices", nil, nil)
	var list []map[string]any
	_ = json.Unmarshal(r.Body.Data, &list)
	if len(list) != 1 {
		t.Errorf("list len %d", len(list))
	}

	if r := h.do("DELETE", path, nil, nil); r.Status != 204 {
		t.Errorf("delete: %d", r.Status)
	}
	for _, m := range []string{"GET", "DELETE"} {
		if r := h.do(m, path, nil, nil); r.Status != 404 || r.Body.Error.Code != "not_found" {
			t.Errorf("%s after delete: %d", m, r.Status)
		}
	}
	if r := h.do("PUT", path, upd, nil); r.Status != 404 {
		t.Errorf("put after delete: %d", r.Status)
	}
}

func TestRequestValidation(t *testing.T) {
	h := newHarness(t, nil)
	h.mustCreate(nas)

	cases := []struct {
		name  string
		body  any
		field string
	}{
		{"missing name", map[string]any{"mac": "aa:bb:cc:dd:ee:02", "interface": "eth0"}, "name"},
		{"bad mac", map[string]any{"name": "x", "mac": "nope", "interface": "eth0"}, "mac"},
		{"bad ip", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02", "ip": "300.1.1.1", "interface": "eth0"}, "ip"},
		{"bad port", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02", "interface": "eth0", "port": 99999}, "port"},
		{"duplicate mac", map[string]any{"name": "x", "mac": "AA:BB:CC:DD:EE:01", "interface": "eth0"}, "mac"},
		{"unknown interface", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02", "interface": "eth9"}, "interface"},
		{"relay to foreign broadcast", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02", "broadcast": "8.8.8.255"}, "broadcast"},
		{"nothing to resolve a network from", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02"}, "broadcast"},
		{"unknown json field", map[string]any{"name": "x", "mac": "aa:bb:cc:dd:ee:02", "interface": "eth0", "admin": true}, "body"},
		{"malformed json", "{not json", "body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := h.do("POST", "/api/v1/devices", tc.body, nil)
			if r.Status != 422 || r.Body.Error == nil || r.Body.Error.Code != "validation_failed" || r.Body.Error.Details[tc.field] == "" {
				t.Fatalf("status %d, error %+v", r.Status, r.Body.Error)
			}
		})
	}

	if r := h.do("GET", "/api/v1/devices/abc", nil, nil); r.Status != 422 {
		t.Errorf("non-numeric id: %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/wake-history?limit=0", nil, nil); r.Status != 422 {
		t.Errorf("bad limit: %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/nope", nil, nil); r.Status != 404 {
		t.Errorf("unknown endpoint: %d", r.Status)
	}
	if r := h.do("PATCH", "/api/v1/devices", nil, nil); r.Status != 405 {
		t.Errorf("wrong method: %d", r.Status)
	}
}

func TestWakeWorkflow(t *testing.T) {
	h := newHarness(t, nil)
	id := h.mustCreate(nas)
	h.checker.SetAlive(false)

	r := h.do("POST", "/api/v1/devices/"+strconv.FormatInt(id, 10)+"/wake", nil, nil)
	if r.Status != 200 {
		t.Fatalf("wake: %d %+v", r.Status, r.Body.Error)
	}
	var res struct {
		PacketSent   bool   `json:"packetSent"`
		Status       string `json:"status"`
		Reachability string `json:"reachability"`
		Broadcast    string `json:"broadcast"`
		Message      string `json:"message"`
	}
	_ = json.Unmarshal(r.Body.Data, &res)
	if !res.PacketSent || res.Status != "sent" || res.Reachability != "pending" || res.Broadcast != "192.168.1.255" {
		t.Errorf("result = %+v", res)
	}

	// A packet was built and addressed correctly.
	if h.sender.Count() != 1 {
		t.Fatalf("packets sent = %d", h.sender.Count())
	}
	sp := h.sender.Sent[0]
	if len(sp.Packet) != 102 || !bytes.Equal(sp.Packet[:6], bytes.Repeat([]byte{0xff}, 6)) ||
		!bytes.Equal(sp.Packet[6:12], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01}) {
		t.Errorf("bad packet % x", sp.Packet[:12])
	}
	if sp.Target.Broadcast.String() != "192.168.1.255" || sp.Target.Port != 9 || sp.Target.SourceIP.String() != "192.168.1.10" {
		t.Errorf("target = %+v", sp.Target)
	}

	// The device never answers: result stays honest ("unknown"), not "off".
	h.waitFor("reachability to settle as unknown", func() bool { return h.history()[0].Reachability == "unknown" })

	// Second wake: device comes up and the record is upgraded to reachable.
	h.checker.SetAlive(true)
	h.do("POST", "/api/v1/devices/"+strconv.FormatInt(id, 10)+"/wake", nil, nil)
	h.waitFor("reachability to become reachable", func() bool { return h.history()[0].Reachability == "reachable" })

	hist := h.history()
	if len(hist) != 2 || hist[0].DeviceName != "NAS" || hist[0].MAC != "aa:bb:cc:dd:ee:01" || hist[0].Status != "sent" {
		t.Errorf("history = %+v", hist)
	}

	// Overview reflects the monitor cache updated by the verifier.
	var ov struct {
		Total, Reachable, NoResponse, Wakes int
	}
	rr := h.do("GET", "/api/v1/overview", nil, nil)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rr.Body.Data, &raw)
	_ = json.Unmarshal(raw["totalDevices"], &ov.Total)
	_ = json.Unmarshal(raw["reachableDevices"], &ov.Reachable)
	_ = json.Unmarshal(raw["noResponseDevices"], &ov.NoResponse)
	_ = json.Unmarshal(raw["totalWakeRequests"], &ov.Wakes)
	if ov.Total != 1 || ov.Reachable != 1 || ov.NoResponse != 0 || ov.Wakes != 2 {
		t.Errorf("overview = %+v", ov)
	}
}

func TestWakeWithoutIPIsNotVerified(t *testing.T) {
	h := newHarness(t, nil)
	id := h.mustCreate(map[string]any{"name": "Bare", "mac": "aa:bb:cc:dd:ee:03", "broadcast": "255.255.255.255"})
	r := h.do("POST", "/api/v1/devices/"+strconv.FormatInt(id, 10)+"/wake", nil, nil)
	if r.Status != 200 {
		t.Fatalf("wake: %d", r.Status)
	}
	if rec := h.history()[0]; rec.Reachability != "not_checked" || rec.Broadcast != "255.255.255.255" {
		t.Errorf("record = %+v", rec)
	}
}

func TestWakeSendFailureIsRecorded(t *testing.T) {
	h := newHarness(t, nil)
	id := h.mustCreate(nas)
	h.sender.Err = errors.New("network is unreachable")

	r := h.do("POST", "/api/v1/devices/"+strconv.FormatInt(id, 10)+"/wake", nil, nil)
	if r.Status != http.StatusBadGateway || r.Body.Error.Code != "wake_failed" {
		t.Fatalf("status %d error %+v", r.Status, r.Body.Error)
	}
	hist := h.history()
	if len(hist) != 1 || hist[0].Status != "failed" || hist[0].Error == "" || hist[0].Reachability != "not_checked" {
		t.Errorf("history = %+v", hist)
	}
}

func TestWakeUnknownDevice(t *testing.T) {
	h := newHarness(t, nil)
	if r := h.do("POST", "/api/v1/devices/42/wake", nil, nil); r.Status != 404 {
		t.Errorf("status %d", r.Status)
	}
	if h.sender.Count() != 0 {
		t.Error("nothing should be sent")
	}
}

func TestInterfaceDisappearingFailsWithoutSending(t *testing.T) {
	// Device is saved against wlan0 while it exists; the interface later goes down.
	cfg := config.Default()
	cfg.Database.Path = ":memory:"
	cfg.Monitor.Enabled = false
	down := wlan0()
	lister := app.FakeLister{eth0(), down}
	sender := &app.FakeSender{}
	a, err := app.New(context.Background(), cfg, app.QuietLogger(), app.Options{Sender: sender, Lister: lister, Checker: &app.FakeChecker{}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	body := `{"name":"x","mac":"aa:bb:cc:dd:ee:05","interface":"wlan0"}`
	res, _ := http.Post(srv.URL+"/api/v1/devices", "application/json", bytes.NewBufferString(body))
	if res.StatusCode != 201 {
		t.Fatalf("create: %d", res.StatusCode)
	}
	lister[1].Up = false // FakeLister is a slice: the app sees the change
	res, _ = http.Post(srv.URL+"/api/v1/devices/1/wake", "application/json", nil)
	if res.StatusCode != 422 || sender.Count() != 0 {
		t.Errorf("status %d, packets %d", res.StatusCode, sender.Count())
	}
}

func TestNetworkInterfacesEndpoint(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.WoL.DefaultInterface = "eth0" })
	r := h.do("GET", "/api/v1/network/interfaces", nil, nil)
	var d struct {
		Interfaces       []network.Interface `json:"interfaces"`
		DefaultInterface string              `json:"defaultInterface"`
	}
	_ = json.Unmarshal(r.Body.Data, &d)
	if r.Status != 200 || len(d.Interfaces) != 2 || d.DefaultInterface != "eth0" || d.Interfaces[0].Addresses[0].Broadcast != "192.168.1.255" {
		t.Errorf("interfaces = %d %+v", r.Status, d)
	}
}

func TestAuthentication(t *testing.T) {
	const token = "0123456789abcdef-secret"
	h := newHarness(t, func(c *config.Config) { c.Auth = config.AuthConfig{Enabled: true, Token: token} })
	h.token = ""

	if r := h.do("GET", "/api/v1/health", nil, nil); r.Status != 200 {
		t.Errorf("health must stay open: %d", r.Status)
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/devices"}, {"POST", "/api/v1/devices"}, {"POST", "/api/v1/devices/1/wake"},
		{"GET", "/api/v1/wake-history"}, {"GET", "/api/v1/network/interfaces"}, {"GET", "/api/v1/settings"},
	} {
		if r := h.do(tc.method, tc.path, nil, nil); r.Status != 401 || r.Header.Get("WWW-Authenticate") == "" {
			t.Errorf("%s %s without token: %d", tc.method, tc.path, r.Status)
		}
	}
	if r := h.do("GET", "/api/v1/devices", nil, map[string]string{"Authorization": "Bearer wrong"}); r.Status != 401 {
		t.Errorf("wrong token: %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/devices", nil, map[string]string{"Authorization": token}); r.Status != 401 {
		t.Errorf("token without Bearer scheme: %d", r.Status)
	}
	if h.sender.Count() != 0 {
		t.Error("unauthenticated wake must not send packets")
	}
	if r := h.do("GET", "/api/v1/devices", nil, map[string]string{"Authorization": "Bearer " + token}); r.Status != 200 {
		t.Errorf("valid token: %d", r.Status)
	}
}

func TestSettingsDoesNotLeakSecrets(t *testing.T) {
	h := newHarness(t, func(c *config.Config) {
		c.Auth = config.AuthConfig{Enabled: true, Token: "0123456789abcdef-secret"}
		c.Database.Path = ":memory:"
	})
	h.token = "0123456789abcdef-secret"
	r := h.do("GET", "/api/v1/settings", nil, nil)
	body, _ := json.Marshal(r.Body.Data)
	if r.Status != 200 || bytes.Contains(body, []byte("secret")) || bytes.Contains(body, []byte(":memory:")) {
		t.Errorf("settings leaked data: %s", body)
	}
}

func TestHostAndOriginProtection(t *testing.T) {
	h := newHarness(t, nil)
	if r := h.do("GET", "/api/v1/health", nil, map[string]string{"Host": "evil.example.com"}); r.Status != 403 {
		t.Errorf("foreign Host: %d", r.Status)
	}
	if r := h.do("GET", "/api/v1/health", nil, map[string]string{"Host": "localhost:8080"}); r.Status != 200 {
		t.Errorf("localhost Host: %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/devices", nas, map[string]string{"Origin": "http://evil.example.com"}); r.Status != 403 {
		t.Errorf("cross-origin POST: %d", r.Status)
	}
	if r := h.do("POST", "/api/v1/devices", nas, map[string]string{"Origin": h.srv.URL}); r.Status != 201 {
		t.Errorf("same-origin POST: %d", r.Status)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t, nil)
	r := h.do("GET", "/api/v1/health", nil, nil)
	if r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("headers = %v", r.Header)
	}
}
