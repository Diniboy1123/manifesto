package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config represents the root configuration structure
type Config struct {
	// Port for the HTTP server
	HttpPort uint16 `json:"http_port,omitempty"`
	// Port for the HTTPS server
	HttpsPort uint16 `json:"https_port,omitempty"`
	// Address to bind the server to
	BindAddr string `json:"bind_addr"`
	// Directory to save temporary cache files to. Emptied on startup
	SaveDir string `json:"save_dir"`
	// List of channels by group name
	Channels map[string][]Channel `json:"channels"`
	// Map of channels for faster lookups by ID (groupName/channelId)
	channelMap map[string]Channel
	// Whether to serve subtitles in manifest requests (ffmpeg gets stuck with stpp subtitles)
	AllowSubs bool `json:"allow_subs"`
	// List of users for authentication (leave empty for no auth)
	Users []User `json:"users"`
	// Duration for caching requests (e.g., "3s")
	CacheDuration JSONDuration `json:"cache_duration"`
	// Path to the log file (if empty, log only to stdout)
	LogPath string `json:"log_path"`
	// GlobalHeaders is a map of HTTP header names to their values.
	// Keys represent header names (e.g., "Authorization"), and values represent their corresponding values (e.g., "Bearer token").
	GlobalHeaders map[string]string `json:"global_headers"`
	// TLSDomainCertMap is a structure that maps domain names to their TLS certificate and key file paths.
	// This is used for serving HTTPS requests with different certificates based on the domain.
	TLSDomainMap []TLSDomainConfig `json:"tls_domain_map"`
	// BogusDomain is a domain used to generate a self signed certificate.
	// This is used when the client does not provide a valid domain.
	// Leave it empty to disable
	BogusDomain string `json:"bogus_domain"`
	// If set to true, the server will return a 204 No Content to any request not made
	// to an existing path.
	HideNotFound bool `json:"hide_not_found"`
	// HttpProxy is the HTTP proxy to use for outgoing requests
	HttpProxy string `json:"http_proxy"`
	// HttpsProxy is the HTTPS proxy to use for outgoing requests
	HttpsProxy string `json:"https_proxy"`
	// NoProxy is a comma-separated list of domains to exclude from proxying
	// like "localhost,.internal". Optional field.
	NoProxy string `json:"no_proxy"`
	// TlsClientInsecure is a flag to disable TLS verification for outgoing requests and proxy connections.
	TlsClientInsecure bool `json:"tls_client_insecure"`
}

// Channel represents a single channel configuration
type Channel struct {
	// Unique identifier for the channel, used in the URLs to identify the channel
	Id string `json:"id"`
	// Reserved for future use to specify the source type of the channel.
	// Currently, it is unused and should be set to "ism" as a placeholder.
	SourceType string `json:"source_type"`
	// Reserved for future use to specify the destination type of the channel.
	// Currently unused, but intended for future support of different output formats. Set it to "mpd" for now.
	DestinationType string `json:"destination_type"`
	// Friendly name for the channel, might be used in the future for display purposes
	Name string `json:"name"`
	// Manifest URL to fetch the stream from
	Url string `json:"url"`
	// If channel is encrypted, this is a list of keys to use for decryption, if left empty, no decryption will be attempted
	Keys []string `json:"keys"`
	// Value to advertise in MPEG-DASH suggestedPresentationDelay attribute
	// useful for live streams where chunks aren't yet available.
	// Set to 0 to disable
	Delay JSONDuration `json:"delay"`
}

// Key represents a keyid and key used for decryption
type Key struct {
	// KeyID is used to identify the track the key is for
	// Usually 16 bytes (hex encoded)
	KeyID []byte
	// Key is the actual key used for decryption
	// It is usually a 16-byte AES key (hex encoded)
	Key []byte
}

// User represents a user for authentication
// If set, users must provide their token in URL pathes to access streams
// If empty, no authentication is required
type User struct {
	// Username is the name of the user
	// It is used for display/logging purposes
	Username string `json:"username"`
	// Token is the token used for authentication
	// Set it to whatever you like, but make sure it is unique and not guessable
	Token string `json:"token"`
}

// JSONDuration is a custom type for smarter JSON unmarshalling of time.Duration
type JSONDuration time.Duration

// TLSDomainConfig represents a TLS domain configuration
type TLSDomainConfig struct {
	// Domain is the domain name for the TLS certificate
	Domain string `json:"domain"`
	// Cert is the path to the TLS certificate file
	Cert string `json:"cert"`
	// Key is the path to the TLS key file
	Key string `json:"key"`
}

var (
	// appConfig holds the current configuration
	appConfig Config
	// ConfigLoaded indicates if the config has been loaded successfully
	ConfigLoaded bool
	// configPath holds the path to the config file
	configPath string
	// configMutex is used to synchronize access to the config
	configMutex sync.RWMutex
)

// LoadConfig loads the configuration from the specified path
func LoadConfig(path string) error {
	configPath = path
	return reloadConfig()
}

// Get returns a copy of the current config (safe for concurrent reads)
func Get() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return appConfig
}

// reloadConfig reloads the configuration from the file, validates it, and updates the global config variable
func reloadConfig() error {
	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	var newConfig Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&newConfig); err != nil {
		return fmt.Errorf("failed to decode config file: %v", err)
	}

	if err := validateConfig(newConfig); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	configMutex.Lock()
	appConfig = newConfig
	appConfig.channelMap = make(map[string]Channel)
	for groupName, channelList := range appConfig.Channels {
		for _, ch := range channelList {
			key := fmt.Sprintf("%s/%s", groupName, ch.Id)
			appConfig.channelMap[key] = ch
		}
	}
	ConfigLoaded = true
	configMutex.Unlock()

	log.Println("Config reloaded successfully")
	return nil
}

// WatchConfig sets up a file watcher to monitor changes to the config file
// and reloads the config when changes are detected
func WatchConfig() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Failed to create config watcher:", err)
		return
	}

	err = watcher.Add(configPath)
	if err != nil {
		fmt.Println("Failed to watch config file:", err)
		return
	}

	go func() {
		var debounceTimer *time.Timer
		var debounceMutex sync.Mutex
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					debounceMutex.Lock()
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					debounceTimer = time.AfterFunc(200*time.Millisecond, func() {
						retryReloadConfig(3, 100*time.Millisecond)
					})
					debounceMutex.Unlock()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("Config watcher error:", err)
			}
		}
	}()
}

// validateConfig checks if the configuration is valid
// and returns an error if any required fields are missing or invalid (since JSON deserialization isn't strict)
func validateConfig(config Config) error {
	if config.HttpPort == 0 && config.HttpsPort == 0 {
		return fmt.Errorf("either http_port or https_port must be greater than 0")
	}
	if config.BindAddr == "" {
		return fmt.Errorf("bind_addr cannot be empty")
	}
	if config.SaveDir == "" {
		return fmt.Errorf("save_dir cannot be empty")
	}
	if config.CacheDuration.Duration() <= 0 {
		return fmt.Errorf("cache_duration must be greater than 0")
	}
	if len(config.TLSDomainMap) > 0 || config.HttpsPort > 0 {
		if config.HttpsPort > 0 && len(config.TLSDomainMap) == 0 {
			return fmt.Errorf("https_port is set, but tls_domain_map must also be provided")
		}
		for _, tlsConfig := range config.TLSDomainMap {
			if tlsConfig.Domain == "" {
				return fmt.Errorf("tls_domain_map contains an entry with an empty domain")
			}
			if tlsConfig.Cert == "" {
				return fmt.Errorf("tls_domain_map entry for domain %s is missing a cert path", tlsConfig.Domain)
			}
			if tlsConfig.Key == "" {
				return fmt.Errorf("tls_domain_map entry for domain %s is missing a key path", tlsConfig.Domain)
			}
			if _, err := os.Stat(tlsConfig.Cert); os.IsNotExist(err) {
				return fmt.Errorf("tls cert file %s does not exist for domain %s", tlsConfig.Cert, tlsConfig.Domain)
			}
			if _, err := os.Stat(tlsConfig.Key); os.IsNotExist(err) {
				return fmt.Errorf("tls key file %s does not exist for domain %s", tlsConfig.Key, tlsConfig.Domain)
			}
		}
	}

	return nil
}

// retryReloadConfig attempts to reload the config a specified number of times with a delay between attempts.
// This is a hacky cross-platform to handle partial writes of the config file.
func retryReloadConfig(retries int, delay time.Duration) {
	for i := 0; i < retries; i++ {
		err := reloadConfig()
		if err == nil {
			return
		}
		if i == retries-1 {
			fmt.Println("Error reloading config after retries:", err)
			return
		}
		time.Sleep(delay)
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface for JSONDuration
// It allows JSON unmarshalling of time.Duration in a more user-friendly format
// (e.g., "3s" instead of 3 seconds in nanoseconds)
func (d *JSONDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = JSONDuration(duration)
	return nil
}

// Duration converts JSONDuration to time.Duration
func (d JSONDuration) Duration() time.Duration {
	return time.Duration(d)
}

// GetKey retrieves a key by its keyId from the channel's keys
func (c Channel) GetKey(keyID []byte) ([]byte, error) {
	for _, rawKey := range c.Keys {
		keyId, parsedKey, err := parseKey(rawKey)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(keyId, keyID) {
			return parsedKey, nil
		}
	}
	return nil, fmt.Errorf("key not found")
}

// parseKey parses a key string in the format "keyId:keyData"
// and returns the key ID and key data as byte slices
func parseKey(key string) (keyID []byte, keyData []byte, err error) {
	if key == "" {
		return nil, nil, fmt.Errorf("key cannot be empty")
	}

	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid key format, expected 'keyId:keyData'")
	}

	keyID, err = hex.DecodeString(parts[0])
	if err != nil || len(keyID) != 16 {
		return nil, nil, fmt.Errorf("invalid key ID, must be a 16-byte hex string")
	}

	keyData, err = hex.DecodeString(parts[1])
	if err != nil || len(keyData) != 16 {
		return nil, nil, fmt.Errorf("invalid key data, must be a 16-byte hex string")
	}

	return keyID, keyData, nil
}

// GetChannel retrieves a channel by group and ID
func (c Config) GetChannel(group, id string) (Channel, bool) {
	key := fmt.Sprintf("%s/%s", group, id)
	channel, exists := c.channelMap[key]
	return channel, exists
}
