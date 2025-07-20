package config

import (
	"strings"
	"testing"
)

func TestValidateChannel(t *testing.T) {
	tests := []struct {
		name        string
		channel     Channel
		groupName   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid ISM channel",
			channel: Channel{
				Id:              "test-ism",
				SourceType:      "ism",
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.ism",
			},
			groupName:   "test-group",
			expectError: false,
		},
		{
			name: "valid MPD channel",
			channel: Channel{
				Id:              "test-mpd",
				SourceType:      "mpd",
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.mpd",
			},
			groupName:   "test-group",
			expectError: false,
		},
		{
			name: "valid channel with default source type",
			channel: Channel{
				Id:              "test-default",
				SourceType:      "", // Should default to ISM
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.ism",
			},
			groupName:   "test-group",
			expectError: false,
		},
		{
			name: "valid channel with default destination type",
			channel: Channel{
				Id:              "test-default-dest",
				SourceType:      "mpd",
				DestinationType: "", // Should default to MPD
				Url:             "https://example.com/manifest.mpd",
			},
			groupName:   "test-group",
			expectError: false,
		},
		{
			name: "invalid source type",
			channel: Channel{
				Id:              "test-invalid",
				SourceType:      "invalid",
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.mpd",
			},
			groupName:   "test-group",
			expectError: true,
			errorMsg:    "unsupported source_type",
		},
		{
			name: "invalid destination type",
			channel: Channel{
				Id:              "test-invalid-dest",
				SourceType:      "mpd",
				DestinationType: "invalid",
				Url:             "https://example.com/manifest.mpd",
			},
			groupName:   "test-group",
			expectError: true,
			errorMsg:    "unsupported destination_type",
		},
		{
			name: "missing channel ID",
			channel: Channel{
				Id:              "",
				SourceType:      "mpd",
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.mpd",
			},
			groupName:   "test-group",
			expectError: true,
			errorMsg:    "channel id cannot be empty",
		},
		{
			name: "missing URL",
			channel: Channel{
				Id:              "test-no-url",
				SourceType:      "mpd",
				DestinationType: "mpd",
				Url:             "",
			},
			groupName:   "test-group",
			expectError: true,
			errorMsg:    "channel url cannot be empty",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateChannel(test.channel, test.groupName)
			
			if test.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if test.errorMsg != "" && !containsString(err.Error(), test.errorMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", test.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateConfig_WithChannels(t *testing.T) {
	// Test config with valid channels
	validConfig := Config{
		HttpPort:      8080,
		BindAddr:      "0.0.0.0",
		SaveDir:       "/tmp",
		CacheDuration: JSONDuration(3000000000), // 3 seconds in nanoseconds
		Channels: map[string][]Channel{
			"test-group": {
				{
					Id:              "ism-channel",
					SourceType:      "ism",
					DestinationType: "mpd",
					Url:             "https://example.com/manifest.ism",
				},
				{
					Id:              "mpd-channel",
					SourceType:      "mpd",
					DestinationType: "mpd",
					Url:             "https://example.com/manifest.mpd",
				},
			},
		},
	}

	err := validateConfig(validConfig)
	if err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}

	// Test config with invalid channel
	invalidConfig := validConfig
	invalidConfig.Channels = map[string][]Channel{
		"test-group": {
			{
				Id:              "invalid-channel",
				SourceType:      "invalid",
				DestinationType: "mpd",
				Url:             "https://example.com/manifest.mpd",
			},
		},
	}

	err = validateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected invalid config to fail validation")
	}
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}