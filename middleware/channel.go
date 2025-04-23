package middleware

import (
	"context"
	"net/http"

	"github.com/Diniboy1123/manifesto/config"
)

// ChannelMiddleware extracts the channel ID from the request URL and retrieves the corresponding channel configuration.
// It checks if the request method is GET or HEAD and validates the channel ID.
// If the channel ID is not found or invalid, it returns a 404 Not Found error.
// If the channel is found, it stores the channel in the request context and calls the next handler.
//
// The channel information is stored in the request context under the key "channel".
func ChannelMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		groupId := r.PathValue("groupId")
		if groupId == "" {
			http.Error(w, "Group ID not found", http.StatusBadRequest)
			return
		}

		channelId := r.PathValue("channelId")
		if channelId == "" {
			http.Error(w, "Channel ID not found", http.StatusBadRequest)
			return
		}

		channel, ok := config.Get().GetChannel(groupId, channelId)
		if !ok {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), "channel", channel)
		next(w, r.WithContext(ctx))
	}
}
