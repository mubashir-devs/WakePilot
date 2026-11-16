// Package api is the HTTP layer: routing, validation, auth and JSON encoding.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"wakepilot/internal/device"
	"wakepilot/internal/network"
	"wakepilot/internal/service"
)

// Success: {"data": ...}. Failure: {"error": {"code", "message", "details"?}}.
type envelope struct {
	Data  any        `json:"data,omitempty"`
	Error *errorBody `json:"error,omitempty"`
	Meta  any        `json:"meta,omitempty"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, envelope{Data: data})
}

func fail(w http.ResponseWriter, status int, code, msg string, details map[string]string) {
	writeJSON(w, status, envelope{Error: &errorBody{Code: code, Message: msg, Details: details}})
}

// writeError maps domain errors to HTTP responses. Unexpected errors are logged
// but their text is never sent to the client.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr device.ValidationError
	switch {
	case errors.As(err, &verr):
		fail(w, http.StatusUnprocessableEntity, "validation_failed", "One or more fields are invalid.", verr)
	case errors.Is(err, device.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found", "Resource not found.", nil)
	case errors.Is(err, service.ErrWakeConfig), network.IsConfigError(err):
		fail(w, http.StatusUnprocessableEntity, "network_config_invalid", err.Error(), nil)
	case errors.Is(err, service.ErrWakeSend):
		fail(w, http.StatusBadGateway, "wake_failed", err.Error(), nil)
	default:
		s.log.LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
		fail(w, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
	}
}

const maxBody = 1 << 20

// decode reads a JSON body strictly: unknown fields and trailing data are errors.
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return device.ValidationError{"body": "request body too large"}
		}
		return device.ValidationError{"body": "invalid JSON: " + err.Error()}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return device.ValidationError{"body": "unexpected data after JSON object"}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, device.ValidationError{"id": "must be a positive integer"}
	}
	return id, nil
}

func queryInt(r *http.Request, key string, def, min, max int) (int, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, device.ValidationError{key: "must be an integer between " + strconv.Itoa(min) + " and " + strconv.Itoa(max)}
	}
	return n, nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
