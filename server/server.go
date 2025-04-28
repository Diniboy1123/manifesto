package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	cfg := config.Get()

	mux := http.NewServeMux()

	if len(cfg.Users) > 0 {
		mux.HandleFunc("GET /{token}/stream/{groupId}/{channelId}/manifest.mpd", buildChain(handlers.DashManifestHandler))
		mux.HandleFunc("GET /{token}/stream/{groupId}/{channelId}/{qualityId}/init.mp4", buildChain(handlers.InitHandler))
		mux.HandleFunc("GET /{token}/stream/{groupId}/{channelId}/{qualityId}/{time}/{rest...}", buildChain(handlers.SegmentHandler))
	} else {
		mux.HandleFunc("GET /stream/{groupId}/{channelId}/manifest.mpd", buildChain(handlers.DashManifestHandler))
		mux.HandleFunc("GET /stream/{groupId}/{channelId}/{qualityId}/init.mp4", buildChain(handlers.InitHandler))
		mux.HandleFunc("GET /stream/{groupId}/{channelId}/{qualityId}/{time}/{rest...}", buildChain(handlers.SegmentHandler))
	}

	if cfg.HideNotFound {
		mux.HandleFunc("/", handlers.NotFoundHandler)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	middleware.InitLogger(ctx)
	defer middleware.ShutdownLogger()

	var servers []*http.Server

	if cfg.HttpPort != 0 {
		addr := net.JoinHostPort(cfg.BindAddr, strconv.Itoa(int(cfg.HttpPort)))
		srv := &http.Server{
			Addr:    addr,
			Handler: mux,
		}
		servers = append(servers, srv)
		go func() {
			log.Printf("manifesto listening on HTTP %s", addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()
	} else {
		log.Println("HTTP server is disabled")
	}

	if cfg.HttpsPort != 0 {
		addr := net.JoinHostPort(cfg.BindAddr, strconv.Itoa(int(cfg.HttpsPort)))
		srv := &http.Server{
			Addr:    addr,
			Handler: mux,
		}
		servers = append(servers, srv)
		go func(srv *http.Server) {
			log.Printf("manifesto listening on HTTPS %s", addr)
			startHTTPSListener(srv)
		}(srv)
	} else {
		log.Println("HTTPS server is disabled")
	}

	<-ctx.Done()
	log.Println("Shutting down servers...")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(ctxShutdown); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}

	log.Println("All servers shut down cleanly")
}
