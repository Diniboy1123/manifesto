package utils

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
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
	refCount int
}

var (
	// cache is a map of URLs to their cached entries
	cache = make(map[string]*cacheEntry)
	// cacheLock is a mutex to synchronize access to the cache
	cacheLock sync.Mutex
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
	cacheDuration := cfg.CacheDuration

	cacheLock.Lock()
	entry, found := cache[url]
	if found && time.Since(entry.timestamp) < cacheDuration.Duration() {
		entry.refCount++
		ready := entry.ready
		cacheLock.Unlock()

		<-ready
		if entry.err != nil {
			return nil, entry.err
		}
		return readResponseFromFile(entry.filePath, url), nil
	}

	fileName := hashURL(url)
	saveDir := cfg.SaveDir

	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		cacheLock.Unlock()
		return nil, err
	}

	filePath := filepath.Join(saveDir, fileName)
	entry = &cacheEntry{
		filePath:  filePath,
		ready:     make(chan struct{}),
		timestamp: time.Now(),
		refCount:  1,
	}
	cache[url] = entry
	cacheLock.Unlock()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}

	req.Header.Set("User-Agent", DEFAULT_USER_AGENT)

	if cfg.GlobalHeaders != nil {
		for k, v := range cfg.GlobalHeaders {
			req.Header.Set(k, v)
		}
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		setEntryError(url, entry, err)
		resp.Body.Close()
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}

	if err := os.WriteFile(filePath, body, 0644); err != nil {
		setEntryError(url, entry, err)
		return nil, err
	}

	cacheLock.Lock()
	entry.timestamp = time.Now()
	cacheLock.Unlock()

	close(entry.ready)

	scheduleCacheCleanup(url, cacheDuration.Duration())

	return readResponseFromFile(entry.filePath, url), nil
}

// setEntryError sets the error for a cache entry and cleans up the cache.
func setEntryError(url string, entry *cacheEntry, err error) {
	entry.err = err
	close(entry.ready)
	cacheLock.Lock()
	delete(cache, url)
	cacheLock.Unlock()
}

// hashURL generates a SHA-1 hash of the URL to use as a filename.
//
// Use of SHA-1 is generally discouraged, but we need speed and collisions will be rare.
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
				cacheLock.Lock()
				if entry, exists := cache[url]; exists {
					entry.refCount--
				}
				cacheLock.Unlock()
			},
		},
		Header: make(http.Header),
	}
}

// scheduleCacheCleanup schedules a cleanup of the cache entry after the specified delay.
// It runs in a separate goroutine and checks if the entry is expired and has no active references.
// If so, it deletes the entry from the cache and removes the file from disk.
// The cleanup is done periodically based on the configured cache duration.
// The cleanup goroutine will exit if the entry is no longer in the cache.
//
// It is important to note that the cleanup will not happen immediately after the delay,
// but rather at the next scheduled interval, which may be longer than the delay.
// This is to avoid excessive CPU usage and to allow for multiple entries to be cleaned up
// in a single pass.
func scheduleCacheCleanup(url string, delay time.Duration) {
	go func() {
		for {
			time.Sleep(delay)

			cacheLock.Lock()
			entry, exists := cache[url]
			if !exists {
				cacheLock.Unlock()
				return
			}
			if time.Since(entry.timestamp) >= delay && entry.refCount == 0 {
				delete(cache, url)
				cacheLock.Unlock()
				os.Remove(entry.filePath)
				return
			}
			cacheLock.Unlock()
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
		log.Println("Error reading cache directory:", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(saveDir, file.Name())

		err := os.Remove(filePath)
		if err != nil {
			log.Println("Error removing cache file:", err)
		}
	}

	return nil
}
