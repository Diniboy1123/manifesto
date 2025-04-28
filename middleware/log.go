package middleware

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Diniboy1123/manifesto/config"
)

var (
	// logMu protects access to the current logger and file
	// to ensure thread-safe operations.
	logMu sync.RWMutex
	// currPath is the current log file path.
	currPath string
	// currLogger is the current logger instance.
	currLogger *log.Logger
	// currFile is the current log file instance.
	currFile *os.File
	// logChan is the channel for logging messages.
	logChan chan string
	// shutdownOnce ensures that the logger is shut down only once.
	shutdownOnce sync.Once
	// logWorkerDone is a channel to signal when the logging worker is done.
	logWorkerDone chan struct{}
)

// LogRequestMiddleware logs incoming HTTP requests.
// It logs the client's IP address, user agent, request path, and user information (if available).
// It also handles log file rotation based on the configured log path.
// The log file is created if it doesn't exist, and the log entries are appended to it.
// The log entries are formatted with a timestamp and the relevant request information.
// This middleware is thread-safe and can handle concurrent requests by using a buffered channel.
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

		select {
		case logChan <- logLine:
		default:
			fmt.Fprintln(os.Stderr, "log channel full, dropping log")
		}

		next.ServeHTTP(w, r)
	})
}

// InitLogger initializes the logger and starts a background goroutine to process log messages.
// It creates a buffered channel for log messages and periodically checks for log file rotation.
// Log messages are written to the current log file and standard output.
// The logger shuts down gracefully when the context is canceled, ensuring all logs are flushed.
func InitLogger(ctx context.Context) {
	logChan = make(chan string, 1000)
	logWorkerDone = make(chan struct{})

	go func() {
		defer close(logWorkerDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					flushAndClose()
					return
				}
				writeLog(line)

			case <-ticker.C:
				checkLogPath()

			case <-ctx.Done():
				flushAndClose()
				return
			}
		}
	}()
}

// ShutdownLogger gracefully shuts down the logger by closing the log channel
// and waiting for the log worker to finish. It ensures that this process
// happens only once, even if ShutdownLogger is called multiple times.
func ShutdownLogger() {
	shutdownOnce.Do(func() {
		close(logChan)
		<-logWorkerDone
	})
}

// flushAndClose ensures that any buffered log messages are written to the current log file
// and then closes the file. This is crucial for preserving all log entries before shutdown.
func flushAndClose() {
	logMu.Lock()
	defer logMu.Unlock()

	if currFile != nil {
		_ = currFile.Sync()
		_ = currFile.Close()
		currFile = nil
		currLogger = nil
	}
}

// writeLog writes a log message to the current log file and standard output.
// It uses the current logger instance to write the log message if available.
// The log message is also printed to the standard output for visibility.
func writeLog(line string) {
	logMu.RLock()
	logger := currLogger
	logMu.RUnlock()

	log.Println(line)
	if logger != nil {
		logger.Println(line)
	}
}

// checkLogPath verifies if the log file path has changed and rotates the log file accordingly.
// If the path has changed, it closes the current log file (if open) and creates a new one.
// The new log file is created if it doesn't exist, and log entries are appended to it.
func checkLogPath() {
	logPath := config.Get().LogPath

	logMu.RLock()
	needsRotate := logPath != currPath
	logMu.RUnlock()

	if needsRotate {
		logMu.Lock()
		defer logMu.Unlock()

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
}
