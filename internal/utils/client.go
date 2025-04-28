package utils

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Diniboy1123/manifesto/config"
)

// Default user agent to use for HTTP requests
const DEFAULT_USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"

// cacheEntry represents a cached HTTP response on disk
// and its associated metadata.
type cacheEntry struct {
	// File path where the response is cached
	filePath string
	// Error encountered during the request (if any)
	err error
	// Timestamp of the last successful request to this URL
	timestamp time.Time
	// Channel to signal when the request is ready
	ready chan struct{}
	// Reference count for the number of active requests using this entry, used for cleanup
	refCount int32
	// know when to close channel
	once sync.Once
}

var (
	// cache is a thread-safe map to store cached responses
	cache = sync.Map{}
)

// trackedBody wraps an io.ReadCloser and tracks its usage.
type trackedBody struct {
	io.ReadCloser
	onClose func()
}

// Read reads data from the wrapped ReadCloser and calls onClose when done.
func (tb *trackedBody) Close() error {
	err := tb.ReadCloser.Close()
	if tb.onClose != nil {
		tb.onClose()
	}
	return err
}

// DoRequest performs an HTTP request and caches the response on disk.
// It returns the cached response if available and not expired.
// If the response is not cached or expired, it performs a new request,
// caches the response, and returns it.
//
// The cache duration and global headers are configurable via the config package.
// The function is thread-safe and handles concurrent requests to the same URL.
// The cache is cleaned up periodically based on the configured cache duration.
func DoRequest(method, url string, headers map[string]string) (*http.Response, error) {
	cfg := config.Get()
	cacheDuration := cfg.CacheDuration.Duration()

	if entryAny, found := cache.Load(url); found {
		entry := entryAny.(*cacheEntry)

		if time.Since(entry.timestamp) >= cacheDuration {
			// Cache expired, trigger a fresh download
			entry.refCount++
			<-entry.ready
			if entry.err != nil {
				return nil, entry.err
			}

			os.Remove(entry.filePath)
			return fetchAndCacheNewResponse(method, url, headers, entry)
		}

		// Cache is valid
		entry.refCount++
		<-entry.ready
		if entry.err != nil {
			return nil, entry.err
		}
		return readResponseFromFile(entry.filePath, url), nil
	}

	return fetchAndCacheNewResponse(method, url, headers, nil)
}

// fetchAndCacheNewResponse is a helper function that performs a new HTTP request,
// caches the response on disk, and returns the response.
// It creates a new cache entry if one does not exist.
// It also handles errors and cleans up the cache entry if the request fails.
func fetchAndCacheNewResponse(method, url string, headers map[string]string, entry *cacheEntry) (*http.Response, error) {
	cfg := config.Get()
	saveDir := cfg.SaveDir
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create saveDir: %w", err)
	}

	if entry == nil {
		entry = &cacheEntry{
			ready:     make(chan struct{}),
			timestamp: time.Now(),
			refCount:  1,
		}
	}

	cache.Store(url, entry)
	defer func() {
		entry.once.Do(func() {
			close(entry.ready)
		})
	}()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}

	req.Header.Set("User-Agent", DEFAULT_USER_AGENT)
	for k, v := range cfg.GlobalHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := GetProxyClient().Do(req)
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		setEntryError(url, entry, fmt.Errorf("bad status: %s", resp.Status))
		return nil, entry.err
	}

	filePath := filepath.Join(saveDir, hashURL(url))
	file, err := os.Create(filePath)
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}

	entry.timestamp = time.Now()
	entry.filePath = filePath

	return readResponseFromFile(filePath, url), nil
}

// setEntryError sets the error for a cache entry and cleans up the cache.
// It closes the ready channel to signal that the request is done.
// It also removes the cached file if it exists.
// This function is called when an error occurs during the request.
// It is thread-safe and ensures that the cache entry is cleaned up properly.
// It also handles the case where the entry is nil, in which case it does nothing.
func setEntryError(url string, entry *cacheEntry, err error) {
	if entry != nil {
		entry.err = err
		select {
		case entry.ready <- struct{}{}:
		default:
			entry.once.Do(func() {
				close(entry.ready)
			})
		}
		cache.Delete(url)

		if entry.filePath != "" {
			_ = os.Remove(entry.filePath)
		}
	}
}

// hashURL generates a SHA-1 hash of the URL to use as a filename.
//
// Note: Use of SHA-1 is generally discouraged, but we need speed and collisions will be rare.
func hashURL(url string) string {
	h := sha1.Sum([]byte(url))
	return hex.EncodeToString(h[:])
}

// readResponseFromFile reads the cached response from the file and returns it as an http.Response.
// It also decrements the reference count for the cache entry when the response is closed.
// If the file cannot be opened, it returns an error response.
func readResponseFromFile(filePath string, url string) *http.Response {
	f, err := os.Open(filePath)
	if err != nil {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(bytes.NewReader([]byte("error reading cache file"))),
		}
	}

	return &http.Response{
		StatusCode: 200,
		Body: &trackedBody{
			ReadCloser: f,
			onClose: func() {
				if entryAny, ok := cache.Load(url); ok {
					entry := entryAny.(*cacheEntry)
					entry.refCount--
				}
			},
		},
		Header: make(http.Header),
	}
}

// StartCleanupLoop starts a goroutine that periodically cleans up the cache.
// It checks the cache entries and removes any that have expired and are not in use.
// The cleanup interval is determined by the cache duration configured in the config package.
//
// The cleanup loop runs indefinitely until the program exits. Call this function
// at startup.
func StartCleanupLoop() {
	go func() {
		cacheDuration := config.Get().CacheDuration.Duration()

		ticker := time.NewTicker(cacheDuration)
		defer ticker.Stop()

		for range ticker.C {
			cache.Range(func(key, value any) bool {
				url := key.(string)
				entry := value.(*cacheEntry)

				if time.Since(entry.timestamp) >= cacheDuration && entry.refCount <= 0 {
					cache.Delete(url)
					_ = os.Remove(entry.filePath)
				}
				return true
			})
		}
	}()
}

// CleanCacheDir cleans up the cache directory by removing all files in it.
// It does not remove any directories, only files.
// It logs any errors encountered during the cleanup process.
//
// This function is useful for clearing the cache manually. Call it at startup.
func CleanCacheDir() error {
	saveDir := config.Get().SaveDir
	files, err := os.ReadDir(saveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("error reading cache directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := filepath.Join(saveDir, file.Name())
		_ = os.Remove(filePath)
	}
	return nil
}
