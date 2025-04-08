package middleware

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Diniboy1123/manifesto/config"
)

var (
	// mu is a mutex to ensure thread-safe access to the log file.
	mu sync.Mutex
	// currPath, currLogger, and currFile are used to manage the current log file.
	currPath string
	// currLogger is the logger instance used for logging.
	currLogger *log.Logger
	// currFile is the current log file being used.
	currFile *os.File
)

// LogRequestMiddleware logs incoming HTTP requests.
// It logs the client's IP address, user agent, request path, and user information (if available).
// It also handles log file rotation based on the configured log path.
// The log file is created if it doesn't exist, and the log entries are appended to it.
// The log entries are formatted with a timestamp and the relevant request information.
// The middleware is thread-safe and ensures that only one goroutine writes to the log file at a time.
//
// The log file is closed when the server shuts down or when the log path changes.
func LogRequestMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		ua := r.UserAgent()

		token := r.PathValue("token")
		path := r.URL.Path
		if token != "" {
			path = strings.Replace(path, token, "***", 1)
		}

		u := r.Context().Value("user")
		userInfo := ""
		if u != nil {
			userInfo = " user=" + u.(*config.User).Username
		}

		logLine := fmt.Sprintf("IP=%s%s path=%q user-agent=%q", ip, userInfo, path, ua)

		log.Println(logLine)

		logPath := config.Get().LogPath

		mu.Lock()
		defer mu.Unlock()

		if logPath != currPath {
			if currFile != nil {
				_ = currFile.Close()
			}
			currPath = logPath
			currLogger = nil
			currFile = nil

			if logPath != "" {
				_ = os.MkdirAll(filepath.Dir(logPath), 0755)
				file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					log.Printf("Warning: could not open log file %s: %v", logPath, err)
				} else {
					currFile = file
					currLogger = log.New(file, "", log.LstdFlags)
				}
			}
		}

		if currLogger != nil {
			currLogger.Println(logLine)
		}

		next.ServeHTTP(w, r)
	})
}
