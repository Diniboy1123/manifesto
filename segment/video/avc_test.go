package video

import (
	"encoding/hex"
	"testing"
)

func TestCodecPrivateDataToSPSPPS(t *testing.T) {
	codecPrivateData := "00000001674d40209e5281806f60284040405000000300100000064e00000d1f400068fa3f13e0a00000000168ef7520"
	expectedSPS := "674d40209e5281806f60284040405000000300100000064e00000d1f400068fa3f13e0a0"
	expectedPPS := "68ef7520"

	spsNALUs, ppsNALUs, err := CodecPrivateDataToSPSPPS(codecPrivateData)
	if err != nil {
		t.Fatalf("Failed to convert codecPrivateData to SPS/PPS: %v", err)
	}

	if len(spsNALUs) == 0 || len(ppsNALUs) == 0 {
		t.Fatal("SPS or PPS NALUs are empty")
	}
	if hex.EncodeToString(spsNALUs[0]) != expectedSPS {
		t.Fatalf("Expected SPS NALU %s, got %s", expectedSPS, hex.EncodeToString(spsNALUs[0]))
	}
	if hex.EncodeToString(ppsNALUs[0]) != expectedPPS {
		t.Fatalf("Expected PPS NALU %s, got %s", expectedPPS, hex.EncodeToString(ppsNALUs[0]))
	}
}
