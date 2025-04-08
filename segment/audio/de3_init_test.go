package audio

import (
	"encoding/hex"
	"testing"
)

func TestExtractDolbyDigitalPlusInfo(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"00063F000000AF87FBA7022DFB42A4D405CD93843BDD0700200F00", "0700200f00"},
		{"000603000000AF87FBA7022DFB42A4D405CD93843BDD0600200400", "0600200400"},
	}

	for _, test := range tests {
		inputBytes, err := hex.DecodeString(test.input)
		if err != nil {
			t.Fatalf("Failed to decode input hex string: %v", err)
		}

		result, err := extractDolbyDigitalPlusInfo(inputBytes)
		if err != nil {
			t.Fatalf("Failed to extract Dolby Digital Plus info: %v", err)
		}
		if hex.EncodeToString(result) != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, hex.EncodeToString(result))
		}
	}
}
