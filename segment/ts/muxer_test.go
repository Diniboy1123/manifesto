package ts

import (
	"bytes"
	"testing"
)

func TestAvccToAnnexB(t *testing.T) {
	// AVCC format: 4-byte length prefix + NALU data
	avcc := []byte{
		0x00, 0x00, 0x00, 0x03, // length = 3
		0x65, 0x88, 0x84, // NALU data (IDR slice)
		0x00, 0x00, 0x00, 0x02, // length = 2
		0x06, 0x05, // NALU data (SEI)
	}

	result := avccToAnnexB(avcc)

	// Expected Annex B: start code + NALU + start code + NALU
	expected := []byte{
		0x00, 0x00, 0x00, 0x01, // start code
		0x65, 0x88, 0x84, // NALU
		0x00, 0x00, 0x00, 0x01, // start code
		0x06, 0x05, // NALU
	}

	if !bytes.Equal(result, expected) {
		t.Errorf("avccToAnnexB mismatch\ngot:  %x\nwant: %x", result, expected)
	}
}

func TestAvccToAnnexBEmpty(t *testing.T) {
	result := avccToAnnexB([]byte{})
	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input, got %d bytes", len(result))
	}
}

func TestAvccToAnnexBTruncated(t *testing.T) {
	// Truncated input (length says 10 bytes but only 3 available)
	avcc := []byte{0x00, 0x00, 0x00, 0x0A, 0x65, 0x88, 0x84}
	result := avccToAnnexB(avcc)
	// Should stop without panicking
	if len(result) != 0 {
		t.Errorf("Expected empty result for truncated input, got %d bytes", len(result))
	}
}

func TestPrependParameterSets(t *testing.T) {
	sps := [][]byte{{0x67, 0x4d, 0x40, 0x20}}
	pps := [][]byte{{0x68, 0xef, 0x75, 0x20}}
	annexB := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88}

	result := prependParameterSets(sps, pps, annexB)

	// Should start with start code + SPS, then start code + PPS, then original data
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	if !bytes.HasPrefix(result, startCode) {
		t.Error("Result should start with start code")
	}
	if !bytes.Contains(result, sps[0]) {
		t.Error("Result should contain SPS")
	}
	if !bytes.Contains(result, pps[0]) {
		t.Error("Result should contain PPS")
	}
	if !bytes.HasSuffix(result, annexB) {
		t.Error("Result should end with original annex B data")
	}
}

func TestCreateADTSFrame(t *testing.T) {
	aacData := []byte{0x01, 0x02, 0x03, 0x04} // 4 bytes of AAC data
	objectType := byte(2)                       // AAC-LC
	sampleRateIndex := byte(3)                  // 48000 Hz
	channelConfig := byte(2)                    // stereo

	result := createADTSFrame(objectType, sampleRateIndex, channelConfig, aacData)

	// Expected frame length: 7 (ADTS header) + 4 (data) = 11
	if len(result) != 11 {
		t.Fatalf("Expected ADTS frame length 11, got %d", len(result))
	}

	// Check sync word
	if result[0] != 0xFF || (result[1]&0xF0) != 0xF0 {
		t.Error("Invalid ADTS sync word")
	}

	// Check that original data is at the end
	if !bytes.Equal(result[7:], aacData) {
		t.Error("AAC data not found at expected position in ADTS frame")
	}
}

func TestSampleRateToIndex(t *testing.T) {
	tests := []struct {
		rate     int
		expected byte
		ok       bool
	}{
		{48000, 3, true},
		{44100, 4, true},
		{96000, 0, true},
		{8000, 11, true},
		{12345, 0, false}, // unsupported
	}

	for _, tt := range tests {
		idx, ok := sampleRateToIndex(tt.rate)
		if ok != tt.ok {
			t.Errorf("sampleRateToIndex(%d): ok=%v, want ok=%v", tt.rate, ok, tt.ok)
		}
		if ok && idx != tt.expected {
			t.Errorf("sampleRateToIndex(%d) = %d, want %d", tt.rate, idx, tt.expected)
		}
	}
}
