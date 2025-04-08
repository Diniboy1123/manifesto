package middleware

import (
	"context"
	"net/http"

	"github.com/Diniboy1123/manifesto/config"
)

// AuthMiddleware handles user authentication based on the provided token in the URL.
// It checks if the token matches any configured user and adds the user to the request context.
// If no users are configured, it allows access without authentication.
// If the token is missing or invalid, it returns a 401 Unauthorized response.
//
// The user information is stored in the request context under the key "user".
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No users configured? No auth required.
		if len(config.Get().Users) == 0 {
			ctx := context.WithValue(r.Context(), "user", nil)

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		token := r.PathValue("token")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user := getUser(token)
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "user", user)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getUser retrieves the user associated with the provided token.
// It iterates through the configured users and returns the matching user.
// If no matching user is found, it returns nil.
func getUser(token string) *config.User {
	for _, user := range config.Get().Users {
		if user.Token == token {
			return &user
		}
	}
	return nil
}
