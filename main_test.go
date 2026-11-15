package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestIndexReportsServiceAndVersion(t *testing.T) {
	a := newApp("example", "abc123")
	response := get(t, a.handler(), "/")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["service"] != "example" || body["version"] != "abc123" {
		t.Errorf("body = %v, want service example and version abc123", body)
	}
}

func TestHealthzIsOKEvenWhenNotReady(t *testing.T) {
	a := newApp("example", "dev")
	if code := get(t, a.handler(), "/healthz").Code; code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
}

func TestReadyzFollowsReadiness(t *testing.T) {
	a := newApp("example", "dev")
	handler := a.handler()
	if code := get(t, handler, "/readyz").Code; code != http.StatusServiceUnavailable {
		t.Errorf("before ready: status = %d, want 503", code)
	}
	a.ready.Store(true)
	if code := get(t, handler, "/readyz").Code; code != http.StatusOK {
		t.Errorf("when ready: status = %d, want 200", code)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	a := newApp("example", "dev")
	if code := get(t, a.handler(), "/nope").Code; code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestMetricsCountRequestsByRoute(t *testing.T) {
	a := newApp("example", "dev")
	handler := a.handler()
	get(t, handler, "/")
	get(t, handler, "/nope")

	body := get(t, handler, "/metrics").Body.String()
	for _, want := range []string{
		`http_requests_total{code="200",route="GET /{$}"} 1`,
		`http_requests_total{code="404",route="unmatched"} 1`,
		"http_request_duration_seconds_bucket",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics do not contain %q", want)
		}
	}
}
