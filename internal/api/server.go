package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"wakepilot/internal/config"
	"wakepilot/internal/device"
	"wakepilot/internal/network"
	"wakepilot/internal/service"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Config  config.Config
	Version string
	Devices *service.DeviceService
	Wake    *service.WakeService
	Network *network.Service
	Log     *slog.Logger
	// Static is the compiled frontend; may be nil or empty.
	Static fs.FS
}

type Server struct {
	Deps
	log         *slog.Logger
	tokenDigest [32]byte
	allowedHost map[string]bool
}

// New builds the HTTP handler with all middleware applied.
func New(d Deps) http.Handler {
	s := &Server{Deps: d, log: d.Log}
	if d.Config.Auth.Enabled {
		s.tokenDigest = sha256.Sum256([]byte(d.Config.Auth.Token))
	}
	s.allowedHost = map[string]bool{}
	for _, h := range d.Config.Server.AllowedHosts {
		s.allowedHost[strings.ToLower(h)] = true
	}
	if len(s.allowedHost) == 0 && d.Config.Server.IsLoopback() {
		for _, h := range []string{"localhost", "127.0.0.1", "::1"} {
			s.allowedHost[h] = true
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/overview", s.auth(s.overview))
	mux.HandleFunc("GET /api/v1/settings", s.auth(s.settings))
	mux.HandleFunc("POST /api/v1/devices", s.auth(s.createDevice))
	mux.HandleFunc("GET /api/v1/devices", s.auth(s.listDevices))
	mux.HandleFunc("GET /api/v1/devices/{id}", s.auth(s.getDevice))
	mux.HandleFunc("PUT /api/v1/devices/{id}", s.auth(s.updateDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", s.auth(s.deleteDevice))
	mux.HandleFunc("POST /api/v1/devices/{id}/wake", s.auth(s.wakeDevice))
	mux.HandleFunc("POST /api/v1/devices/{id}/check", s.auth(s.checkDevice))
	mux.HandleFunc("GET /api/v1/wake-history", s.auth(s.wakeHistory))
	mux.HandleFunc("GET /api/v1/network/interfaces", s.auth(s.interfaces))
	mux.Handle("GET /", s.static()) // GET-only so wrong methods on API routes yield 405

	return s.recoverer(s.logRequests(s.securityHeaders(s.hostCheck(s.originCheck(jsonMethodErrors(mux))))))
}

// ---- middleware -----------------------------------------------------------

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic in handler", "panic", v, "path", r.URL.Path)
				fail(w, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// hostCheck rejects unexpected Host headers (DNS-rebinding defence).
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.allowedHost) > 0 && !s.allowedHost[strings.ToLower(hostOnly(r.Host))] {
			fail(w, http.StatusForbidden, "forbidden_host", "Host header is not allowed.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originCheck blocks cross-site state-changing requests: if a browser sends an
// Origin, it must match the Host it is talking to.
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					fail(w, http.StatusForbidden, "forbidden_origin", "Cross-origin requests are not allowed.", nil)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if !s.Config.Auth.Enabled {
		return true
	}
	tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return false
	}
	d := sha256.Sum256([]byte(tok))
	return subtle.ConstantTimeCompare(d[:], s.tokenDigest[:]) == 1
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wakepilot"`)
			fail(w, http.StatusUnauthorized, "unauthorized", "A valid bearer token is required.", nil)
			return
		}
		next(w, r)
	}
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

// ---- static frontend ------------------------------------------------------

func (s *Server) static() http.Handler {
	if s.Static == nil {
		s.Static = emptyFS{}
	}
	files := http.FileServerFS(s.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, http.StatusNotFound, "not_found", "Unknown API endpoint.", nil)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Static, p); err != nil {
			// SPA fallback for client-side routes.
			if _, ierr := fs.Stat(s.Static, "index.html"); ierr != nil {
				http.Error(w, "frontend not built: run `npm run build` in frontend/ (or use the Vite dev server)", http.StatusNotFound)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			files.ServeHTTP(w, r2)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// ---- handlers -------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ok(w, http.StatusOK, map[string]any{
		"status": "ok", "version": s.Version, "authRequired": s.Config.Auth.Enabled,
	})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.Wake.Overview(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ok(w, http.StatusOK, o)
}

// settings exposes non-sensitive configuration (never the token or DB path).
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	c := s.Config
	ok(w, http.StatusOK, map[string]any{
		"server":     map[string]any{"host": c.Server.Host, "port": c.Server.Port, "loopbackOnly": c.Server.IsLoopback()},
		"auth":       map[string]any{"enabled": c.Auth.Enabled},
		"wol":        map[string]any{"defaultPort": c.WoL.DefaultPort, "defaultInterface": c.WoL.DefaultInterface, "sendTimeout": c.WoL.SendTimeout.String(), "verifyEnabled": c.WoL.VerifyEnabled, "verifyTimeout": c.WoL.VerifyTimeout.String()},
		"monitor":    map[string]any{"enabled": c.Monitor.Enabled, "interval": c.Monitor.Interval.String(), "ports": c.Monitor.Ports},
		"logging":    map[string]any{"level": c.Logging.Level, "format": c.Logging.Format},
		"version":    s.Version,
		"configFile": "read-only; edit the YAML file and restart",
	})
}

func (s *Server) interfaces(w http.ResponseWriter, r *http.Request) {
	ifs, err := s.Network.List()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if ifs == nil {
		ifs = []network.Interface{}
	}
	ok(w, http.StatusOK, map[string]any{"interfaces": ifs, "defaultInterface": s.Network.DefaultInterface()})
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var in device.Input
	if err := decode(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	d, err := s.Devices.Create(r.Context(), in)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/devices/"+itoa(d.ID))
	ok(w, http.StatusCreated, d)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	ds, err := s.Devices.List(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ok(w, http.StatusOK, ds)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	d, err := s.Devices.Get(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ok(w, http.StatusOK, d)
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var in device.Input
	if err := decode(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	d, err := s.Devices.Update(r.Context(), id, in)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ok(w, http.StatusOK, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.Devices.Delete(r.Context(), id); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	d, err := s.Devices.Check(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ok(w, http.StatusOK, d)
}

// wakeResult spells out what a successful wake does and does not prove.
type wakeResult struct {
	device.WakeRecord
	PacketSent bool   `json:"packetSent"`
	Message    string `json:"message"`
}

func (s *Server) wakeDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	rec, err := s.Wake.Wake(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	msg := "Magic packet sent. This does not confirm the device has powered on."
	if rec.Reachability == device.ReachPending {
		msg += " Checking whether it becomes reachable…"
	}
	ok(w, http.StatusOK, wakeResult{WakeRecord: rec, PacketSent: true, Message: msg})
}

func (s *Server) wakeHistory(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 50, 1, 500)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	offset, err := queryInt(r, "offset", 0, 0, 1<<30)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	f := device.HistoryFilter{Limit: limit, Offset: offset}
	if v := r.URL.Query().Get("deviceId"); v != "" {
		var n int
		if n, err = queryInt(r, "deviceId", 0, 1, 1<<30); err != nil {
			s.writeError(w, r, err)
			return
		}
		id := int64(n)
		f.DeviceID = &id
	}
	recs, total, err := s.Wake.History(r.Context(), f)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: recs, Meta: map[string]int{"total": total, "limit": limit, "offset": offset}})
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

// jsonMethodErrors rewrites the mux's plain-text 405 into the standard JSON error.
func jsonMethodErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&methodErrWriter{ResponseWriter: w}, r)
	})
}

type methodErrWriter struct {
	http.ResponseWriter
	swallow bool
}

func (w *methodErrWriter) WriteHeader(code int) {
	if code == http.StatusMethodNotAllowed && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		w.swallow = true
		fail(w.ResponseWriter, code, "method_not_allowed", "Method not allowed for this endpoint.", nil)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *methodErrWriter) Write(b []byte) (int, error) {
	if w.swallow {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
