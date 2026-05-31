package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Observability struct {
	service  string
	started  time.Time
	requests atomic.Uint64
	logger   *slog.Logger
}

func NewObservability(service string, logger *slog.Logger) *Observability {
	if service == "" {
		service = "api"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Observability{service: service, started: time.Now().UTC(), logger: logger}
}

func (o *Observability) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		o.requests.Add(1)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		o.logger.Info("request", "method", r.Method, "path", safePath(r.URL.Path), "status", recorder.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (o *Observability) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": o.service})
}

func (o *Observability) HandleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": o.service})
}

func (o *Observability) HandleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte("echotrails_api_requests_total " + jsonNumber(o.requests.Load()) + "\n"))
	_, _ = w.Write([]byte("echotrails_api_uptime_seconds " + jsonNumber(uint64(time.Since(o.started).Seconds())) + "\n"))
}

func safePath(path string) string {
	for _, sensitive := range []string{"token", "code", "secret", "lat", "lon"} {
		if strings.Contains(strings.ToLower(path), sensitive) {
			return "[redacted]"
		}
	}
	return path
}

func jsonNumber(value uint64) string {
	bytes, _ := json.Marshal(value)
	return string(bytes)
}
