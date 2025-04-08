package audio

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

// AACInitSegment represents an initialization segment for AAC audio streams.
type AACInitSegment struct {
	segment.BaseInitSegment
}

// CodecPrivateDataToAudioSpecificConfig converts MSS codec private data in hex format to AudioSpecificConfig.
// It decodes the hex string and returns an AudioSpecificConfig object.
func CodecPrivateDataToAudioSpecificConfig(codecPrivateDataHex string) (*aac.AudioSpecificConfig, error) {
	codecPrivateData, err := hex.DecodeString(codecPrivateDataHex)
	if err != nil {
		return nil, err
	}
	if len(codecPrivateData) < 2 {
		return nil, fmt.Errorf("invalid codecPrivateData length")
	}
	return aac.DecodeAudioSpecificConfig(bytes.NewReader(codecPrivateData))
}

// Generate creates an initialization segment for AAC audio streams.
// It sets the audio configuration based on the provided codec private data and
// adds encryption information if a key ID and PSSH data are provided.
// It returns the generated initialization segment and any decryption information.
//
// If an error occurs during the generation process, it returns the error.
//
// The function also sets the language and time scale for the segment.
func (s *AACInitSegment) Generate() (*mp4.InitSegment, mp4.DecryptInfo, error) {
	audioConfig, err := CodecPrivateDataToAudioSpecificConfig(s.CodecPrivateData)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	init := segment.NewBaseInitSegment("audio", s.Lang, s.TimeScale, []string{"iso6", "piff", "mp4a"})
	err = init.Moov.Trak.SetAACDescriptor(audioConfig.ObjectType, audioConfig.SamplingFrequency)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	if s.KeyId != nil && s.Pssh != nil {
		decryptInfo, err := segment.AddPrEncryption(init, s.Key, s.KeyId, s.Pssh)
		return init, decryptInfo, err
	}

	return init, mp4.DecryptInfo{}, nil
}
