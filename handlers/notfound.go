package handlers

import "net/http"

// NotFoundHandler handles requests for non-existent resources.
// It responds with a 204 No Content status code.
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
