package video

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Eyevinn/mp4ff/mp4"
)

// AVCInitSegment represents an initialization segment for AVC video streams.
type AVCInitSegment struct {
	segment.BaseInitSegment
}

// CodecPrivateDataToSPSPPS converts codec private data in hex format to SPS and PPS NALUs.
// It decodes the hex string and splits it into SPS and PPS NALUs.
func CodecPrivateDataToSPSPPS(codecPrivateDataHex string) (spsNALUs [][]byte, ppsNALUs [][]byte, err error) {
	codecPrivateData, err := hex.DecodeString(codecPrivateDataHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode codecPrivateDataHex: %v", err)
	}

	delimiter := []byte{0, 0, 0, 1}
	split := bytes.SplitN(codecPrivateData, delimiter, 3)
	if len(split) < 3 {
		return nil, nil, fmt.Errorf("invalid codecPrivateDataHex format")
	}

	spsNALUs = [][]byte{split[1]}
	ppsNALUs = [][]byte{split[2]}

	return spsNALUs, ppsNALUs, nil
}

// Generate creates an initialization segment for AVC video streams.
// It sets the video configuration based on the provided codec private data and
// adds encryption information if a key ID and PSSH data are provided.
// It returns the generated initialization segment and any decryption information.
//
// If an error occurs during the generation process, it returns the error.
//
// The function also sets the language and time scale for the segment.
func (s *AVCInitSegment) Generate() (*mp4.InitSegment, mp4.DecryptInfo, error) {
	spsNALUs, ppsNALUs, err := CodecPrivateDataToSPSPPS(s.CodecPrivateData)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	init := segment.NewBaseInitSegment("video", s.Lang, s.TimeScale, []string{"iso6", "piff", "avc1"})
	err = init.Moov.Trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, true)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	if s.KeyId != nil && s.Pssh != nil {
		decryptionInfo, err := segment.AddPrEncryption(init, s.Key, s.KeyId, s.Pssh)
		return init, decryptionInfo, err
	}

	return init, mp4.DecryptInfo{}, nil
}
