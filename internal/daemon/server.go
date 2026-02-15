package daemon

import (
	"log/slog"
	"net/http"
	"time"
)

// responseRecorder wraps http.ResponseWriter to capture the status code.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// applyMiddleware wraps the mux with the middleware chain.
func (d *Daemon) applyMiddleware(mux http.Handler) http.Handler {
	// Apply middleware in reverse order (outermost first).
	handler := jsonContentType(mux)
	handler = requestLogger(handler)
	return handler
}

// requestLogger logs method, path, status code, and duration for each request.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rr, r)
		duration := time.Since(start)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rr.statusCode,
			"duration", duration.String(),
		)
	})
}

// jsonContentType sets the Content-Type header to application/json for all responses.
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
