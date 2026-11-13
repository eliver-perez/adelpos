package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/database"
)

func handler(health database.Health) http.Handler {
	return New(func(context.Context) database.Health {
		return health
	}, []string{"tauri://localhost"}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func TestTechnicalEndpoints(t *testing.T) {
	h := handler(database.Health{Postgres: "available", Schema: "compatible"})
	for route, expected := range map[string]int{"/api/v1/health/live": 200, "/api/v1/health/ready": 200, "/api/v1/version": 200, "/api/v1/events": 501, "/missing": 404} {
		r := httptest.NewRequest("GET", route, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("%s: %d", route, w.Code)
		}
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatalf("%s: not JSON", route)
		}
		if w.Header().Get("X-Request-ID") == "" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing response headers")
		}
		for _, secret := range []string{"password", "connection_string", "postgresql://"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("sensitive data exposed")
			}
		}
	}
}

func TestReadinessAndLivenessAreIndependent(t *testing.T) {
	for _, health := range []database.Health{{Postgres: "unavailable", Schema: "unknown"}, {Postgres: "available", Schema: "incompatible"}} {
		h := handler(health)
		for route, status := range map[string]int{"/api/v1/health/live": 200, "/api/v1/health/ready": 503} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", route, nil))
			if w.Code != status {
				t.Fatal("readiness and liveness conflated")
			}
		}
	}
}

func TestCORSAndMethodRestrictions(t *testing.T) {
	h := handler(database.Health{})
	for _, tc := range []struct {
		method, origin string
		expected       int
	}{{"GET", "https://attacker.example", 403}, {"GET", "null", 403}, {"GET", "tauri://localhost", 200}, {"POST", "", 405}, {"OPTIONS", "tauri://localhost", 204}} {
		request := httptest.NewRequest(tc.method, "/api/v1/health/live", nil)
		request.Header.Set("Origin", tc.origin)
		if tc.method == "OPTIONS" {
			request.Header.Set("Access-Control-Request-Method", "GET")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, request)
		if w.Code != tc.expected {
			t.Fatalf("%+v got %d", tc, w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Fatal("wildcard CORS")
		}
	}
}

func TestPanicDoesNotLeakInternals(t *testing.T) {
	h := New(func(context.Context) database.Health {
		panic("postgresql://secret")
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/health/ready", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("panic response is unsafe")
	}
}

// Etapa 9F (§3): /api/v1/version informa el modo de pruebas, falso salvo que el servidor lo declare.
func TestVersionReportsTestMode(t *testing.T) {
	probe := func(context.Context) database.Health {
		return database.Health{Postgres: "available", Schema: "compatible"}
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, expected := range []bool{false, true} {
		h := NewWithOptions(Options{TestMode: expected}, probe, nil, logger)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/version", nil))
		var body struct {
			TestMode *bool `json:"test_mode"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.TestMode == nil {
			t.Fatalf("version sin test_mode: %s", w.Body.String())
		}
		if *body.TestMode != expected {
			t.Fatalf("test_mode %v, se esperaba %v", *body.TestMode, expected)
		}
	}
	w := httptest.NewRecorder()
	handler(database.Health{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/version", nil))
	if !strings.Contains(w.Body.String(), `"test_mode":false`) {
		t.Fatalf("el servidor ordinario debe informar test_mode false: %s", w.Body.String())
	}
}
