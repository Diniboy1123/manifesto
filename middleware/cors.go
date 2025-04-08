package middleware

import (
	"net/http"
)

// CorsMiddleware adds CORS headers to the response.
// It allows requests from any origin and sets the "X-Powered-By" header to "manifesto".
// This middleware should be used for all HTTP handlers to enable CORS support.
func CorsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		w.Header().Set("X-Powered-By", "manifesto")

		next(w, r)
	}
}
