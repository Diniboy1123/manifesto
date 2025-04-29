package middleware

import (
	"context"
	"net/http"
	"time"
)

// CorsMiddleware adds CORS headers to the response.
// It allows requests from any origin and sets the "X-Powered-By" header to "manifesto".
// This middleware should be used for all HTTP handlers to enable CORS support.
func CorsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Note: For sure not the most ideal middleware for this, but it's the first one
		// that gets called, so we can set the start time here.
		ctx := context.WithValue(r.Context(), "reqStartTime", time.Now())

		w.Header().Set("Access-Control-Allow-Origin", "*")

		w.Header().Set("X-Powered-By", "manifesto")

		next(w, r.WithContext(ctx))
	}
}
