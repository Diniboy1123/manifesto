package server

import (
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/handlers"
	"github.com/Diniboy1123/manifesto/middleware"
)

// buildChain constructs a middleware chain for the given handler.
// Add new middleware functions here to apply them in order.
func buildChain(handler http.HandlerFunc) http.HandlerFunc {
	return middleware.CorsMiddleware(
		middleware.AuthMiddleware(
			middleware.LogRequestMiddleware(
				middleware.ChannelMiddleware(handler),
			),
		),
	)
}

// Start initializes and starts the HTTP server.
// It sets up the request multiplexer with the appropriate routes and middleware.
// The server listens on the configured bind address and port.
// If the HTTP port is not configured, the server will not start.
// The server will log the listening address and any errors encountered during startup.
// The server will block until terminated, allowing for graceful shutdown.
// The function also checks if any users are configured and sets up the routes accordingly.
// If no users are configured, the routes will not require authentication.
func Start() {
	mux := http.NewServeMux()

	cfg := config.Get()

	if len(cfg.Users) > 0 {
		mux.HandleFunc("GET /{token}/stream/{channelId}/manifest.mpd", buildChain(handlers.DashManifestHandler))
		mux.HandleFunc("GET /{token}/stream/{channelId}/{qualityId}/init.mp4", buildChain(handlers.InitHandler))
		mux.HandleFunc("GET /{token}/stream/{channelId}/{qualityId}/{time}/{rest...}", buildChain(handlers.SegmentHandler))
	} else {
		mux.HandleFunc("GET /stream/{channelId}/manifest.mpd", buildChain(handlers.DashManifestHandler))
		mux.HandleFunc("GET /stream/{channelId}/{qualityId}/init.mp4", buildChain(handlers.InitHandler))
		mux.HandleFunc("GET /stream/{channelId}/{qualityId}/{time}/{rest...}", buildChain(handlers.SegmentHandler))
	}

	if cfg.HideNotFound {
		mux.HandleFunc("/", handlers.NotFoundHandler)
	}

	if cfg.HttpPort != 0 {
		go func() {
			host := net.JoinHostPort(cfg.BindAddr, strconv.Itoa(int(cfg.HttpPort)))
			log.Printf("manifesto listening on HTTP %s", host)
			if err := http.ListenAndServe(host, mux); err != nil {
				log.Fatalf("Error starting server: %v", err)
			}
		}()
	} else {
		log.Println("HTTP server is disabled")
	}

	if cfg.HttpsPort != 0 {
		go func() {
			addr := net.JoinHostPort(cfg.BindAddr, strconv.Itoa(int(cfg.HttpsPort)))
			startHTTPSListener(addr, mux)
		}()
	} else {
		log.Println("HTTPS server is disabled")
	}

	select {}
}
