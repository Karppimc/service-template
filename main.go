// A small HTTP service on the platform's paved road: health endpoints for the
// probes, Prometheus metrics, JSON logs and a graceful shutdown. Replace the
// index handler with your own routes and keep the rest.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// version is set at build time: -ldflags "-X main.version=<commit>".
var version = "dev"

type app struct {
	name     string
	version  string
	ready    atomic.Bool
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newApp(name, version string) *app {
	a := &app{
		name:     name,
		version:  version,
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests by route and status code.",
		}, []string{"route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration by route.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route"}),
	}
	a.registry.MustRegister(
		a.requests,
		a.duration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return a
}

// handler returns the routes of the service, wrapped in the metrics middleware.
func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.index)
	// Liveness: the process answers. Must not depend on anything outside the process
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Readiness: the service can take traffic. False during startup and shutdown
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !a.ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{}))
	return a.instrument(mux)
}

func (a *app) index(w http.ResponseWriter, _ *http.Request) {
	hostname, _ := os.Hostname()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"service":  a.name,
		"version":  a.version,
		"hostname": hostname,
	}); err != nil {
		slog.Error("writing response", "error", err)
	}
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// instrument counts and times every request. The route label is the matched
// pattern, not the raw path, so the number of label values stays bounded.
func (a *app) instrument(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(recorder, r)

		_, route := next.Handler(r)
		if route == "" {
			route = "unmatched"
		}
		a.requests.WithLabelValues(route, strconv.Itoa(recorder.code)).Inc()
		a.duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	a := newApp(envOr("SERVICE_NAME", "service"), version)
	server := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           a.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Kubernetes sends SIGTERM before it kills the pod
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	go func() {
		slog.Info("listening", "addr", server.Addr, "service", a.name, "version", a.version)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	a.ready.Store(true)

	<-ctx.Done()
	// Fail readiness first, so the pod leaves the Service before connections close
	a.ready.Store(false)
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
}
