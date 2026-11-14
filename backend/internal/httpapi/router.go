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

	"local/ad-el-pos/server/internal/apicontract"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/product"
)

type Probe func(context.Context) database.Health

// capabilities son las funciones que anuncia /api/v1/version. Un cliente comprueba las que necesita en lugar de
// suponerlas por la versión; desde la etapa 9G se agregan las de la API para Android.
var capabilities = []string{
	"technical-health",
	"identity-administration",
	"catalog-administration",
	"exact-price-preview",
	"restaurant-operations",
	"kitchen-preparation",
	"persistent-print-queue",
	"authenticated-sse-v1",
	"client-version-check",
	"openapi-v1",
	"catalog-sync-v1",
	"catalog-image-content",
	"catalog-sold-out",
	"offline-orders-v1",
	"state-recovery-v1",
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
type contextKey struct{}

// Options son los ajustes del servidor que informan las rutas técnicas.
type Options struct {
	// TestMode lo informa /api/v1/version (etapa 9F): los clientes de las pruebas automáticas se niegan a
	// correr contra un servidor que no lo declare.
	TestMode bool
}

func New(probe Probe, origins []string, logger *slog.Logger, application ...http.Handler) http.Handler {
	return NewWithOptions(Options{}, probe, origins, logger, application...)
}

func NewWithOptions(options Options, probe Probe, origins []string, logger *slog.Logger, application ...http.Handler) http.Handler {
	metadata := product.Read()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health/live", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"status": "alive"})
	})
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
		respond(w, 200, map[string]any{
			"product":                metadata.TechnicalID,
			"server_version":         metadata.Version,
			"api_version":            metadata.APIVersion,
			"schema_version":         metadata.SchemaVersion,
			"supported_api_versions": []string{metadata.APIVersion},
			"capabilities":           capabilities,
			"events_available":       len(application) > 0,
			"test_mode":              options.TestMode,
			// Etapa 9G (§5.1): la versión más antigua de la aplicación que acepta este servidor, los tipos de
			// cliente que conoce y dónde está el contrato OpenAPI de la API v1.
			"min_client_version": metadata.MinClientVersion,
			"client_kinds":       ClientKinds,
			"openapi_path":       "/api/v1/openapi.json",
		})
	})
	// Contrato OpenAPI 3.1 de la API v1 para los clientes (etapa 9G, §5.1): el mismo documento que comprueban las
	// pruebas de contrato y que usará la etapa de Android.
	mux.HandleFunc("/api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(apicontract.Document())
	})
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if len(application) > 0 {
			application[0].ServeHTTP(w, r)
			return
		}
		fail(w, r, 501, "EVENTS_NOT_AVAILABLE", "Los eventos en tiempo real aún no están habilitados.")
	})
	if len(application) > 0 {
		mux.Handle("/api/v1/", application[0])
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, 404, "NOT_FOUND", "Ruta no disponible.")
	})
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
		technicalRoute := slices.Contains([]string{"/api/v1/health/live", "/api/v1/health/ready", "/api/v1/version", "/api/v1/events", "/api/v1/openapi.json"}, r.URL.Path)
		if r.Method != http.MethodGet && (technicalRoute || len(application) == 0) {
			w.Header().Set("Allow", "GET, OPTIONS")
			fail(tracked, r, 405, "METHOD_NOT_ALLOWED", "Método no permitido.")
			return
		}
		// Un cliente que se identifica con una versión anterior a la mínima recibe «Actualiza esta terminal»
		// antes de llegar a la aplicación (etapa 9G, §5.1; clientversion.go).
		if problem := checkClient(r, metadata.MinClientVersion, metadata.Version); problem != nil {
			failWithDetails(tracked, r, problem.status, problem.code, problem.message, problem.details)
			return
		}
		mux.ServeHTTP(tracked, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
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

// failWithDetails es fail con datos públicos del rechazo en error.details (versión del cliente y la mínima).
func failWithDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	if details == nil {
		fail(w, r, status, code, message)
		return
	}
	id, _ := r.Context().Value(contextKey{}).(string)
	respond(w, status, map[string]any{"error": map[string]any{
		"code":       code,
		"message":    message,
		"request_id": id,
		"details":    details,
	}})
}
