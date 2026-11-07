package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/product"
)

type Probe func(context.Context) database.Health

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
type contextKey struct{}

func New(probe Probe, origins []string, logger *slog.Logger) http.Handler {
	metadata := product.Read()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "alive"}) })
	mux.HandleFunc("/api/v1/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		h := probe(ctx)
		status, code := "ready", http.StatusOK
		if h.Postgres != "available" || h.Schema != "compatible" {
			status, code = "not_ready", http.StatusServiceUnavailable
		}
		respond(w, code, struct {
			Status string          `json:"status"`
			Checks database.Health `json:"checks"`
		}{status, h})
	})
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"product": metadata.TechnicalID, "server_version": metadata.Version, "api_version": metadata.APIVersion, "schema_version": metadata.SchemaVersion, "supported_api_versions": []string{metadata.APIVersion}, "capabilities": []string{"technical-health"}, "events_available": false})
	})
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, 501, "EVENTS_NOT_AVAILABLE", "Los eventos en tiempo real aún no están habilitados.")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, r, 404, "NOT_FOUND", "Ruta no disponible.") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var token [16]byte
		_, _ = rand.Read(token[:])
		id := hex.EncodeToString(token[:])
		r = r.WithContext(context.WithValue(r.Context(), contextKey{}, id))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		tracked := &responseWriter{ResponseWriter: w, status: 200}
		defer func() {
			if recover() != nil {
				fail(tracked, r, 500, "INTERNAL_ERROR", "No fue posible completar la solicitud.")
				logger.Error("http_panic", "request_id", id)
			}
			// No registrar query strings, cabeceras, cuerpos ni direcciones con credenciales.
			logger.Info("http_request", "method", r.Method, "status", tracked.status, "request_id", id, "duration_ms", time.Since(start).Milliseconds())
		}()
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if origin != "" {
			if !slices.Contains(origins, origin) {
				fail(tracked, r, 403, "ORIGIN_NOT_ALLOWED", "Origen no permitido.")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == http.MethodOptions {
			if origin == "" || r.Header.Get("Access-Control-Request-Method") != "GET" || r.Header.Get("Access-Control-Request-Headers") != "" {
				fail(tracked, r, 403, "PREFLIGHT_REJECTED", "Solicitud de origen no permitida.")
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET")
			tracked.WriteHeader(204)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET, OPTIONS")
			fail(tracked, r, 405, "METHOD_NOT_ALLOWED", "Método no permitido.")
			return
		}
		mux.ServeHTTP(tracked, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id, _ := r.Context().Value(contextKey{}).(string)
	respond(w, status, map[string]any{"error": ErrorBody{code, message, id}})
}
