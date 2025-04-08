package audio

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Eyevinn/mp4ff/mp4"
)

// De3InitSegment represents an initialization segment for Dolby Digital Plus (EAC-3) audio streams.
type De3InitSegment struct {
	segment.BaseInitSegment
}

// DDP_WAVEFORMAT_GUID is the GUID for Dolby Digital Plus (EAC-3) audio format.
// It is used to identify the audio format in the codec private data.
// based on official Microsoft mfapi.h header: https://www.magnumdb.com/search?q=MFAudioFormat_Dolby_DDPlus
var DDP_WAVEFORMAT_GUID = []byte{0xaf, 0x87, 0xfb, 0xa7, 0x02, 0x2d, 0xfb, 0x42, 0xa4, 0xd4, 0x05, 0xcd, 0x93, 0x84, 0x3b, 0xdd}

// extractDolbyDigitalPlusInfo extracts the Dolby Digital Plus (EAC-3) information from the codec private data.
// It checks if the codec private data contains the correct GUID and extracts the relevant information.
// The function returns the extracted information as a byte slice.
//
// If the GUID is not found, it returns an error.
func extractDolbyDigitalPlusInfo(info []byte) ([]byte, error) {
	// based on a really long research that ended up here: https://github.com/axiomatic-systems/Bento4/blob/3bdc891602d19789b8e8626e4a3e613a937b4d35/Source/Python/utils/mp4utils.py#L1047

	if !bytes.Equal(info[6:22], DDP_WAVEFORMAT_GUID) {
		return nil, fmt.Errorf("invalid DDP_WAVEFORMAT_GUID")
	}

	return info[6+len(DDP_WAVEFORMAT_GUID):], nil
}

// CodecPrivateDataToDec3Box converts the codec private data in hex format to a Dec3Box.
// It decodes the hex string and returns a Dec3Box object.
//
// If the codec private data is invalid or cannot be decoded, it returns an error.
func CodecPrivateDataToDec3Box(codecPrivateDataHex string) (*mp4.Dec3Box, error) {
	codecPrivateData, err := hex.DecodeString(codecPrivateDataHex)
	if err != nil {
		return nil, err
	}
	if len(codecPrivateData) < 2 {
		return nil, fmt.Errorf("invalid codecPrivateData length")
	}
	payload, err := extractDolbyDigitalPlusInfo(codecPrivateData)
	if err != nil {
		return nil, err
	}
	box, err := mp4.DecodeDec3(mp4.BoxHeader{}, 0, bytes.NewReader(payload))
	if err != nil || box == nil {
		return nil, fmt.Errorf("failed to decode Dec3Box: %v", err)
	}
	return box.(*mp4.Dec3Box), nil
}

// Generate creates an initialization segment for Dolby Digital Plus (EAC-3) audio streams.
// It sets the audio configuration based on the provided codec private data and
// adds encryption information if a key ID and PSSH data are provided.
// It returns the generated initialization segment and any decryption information.
//
// If an error occurs during the generation process, it returns the error.
//
// The function also sets the language and time scale for the segment.
func (s *De3InitSegment) Generate() (*mp4.InitSegment, mp4.DecryptInfo, error) {
	dec3Box, err := CodecPrivateDataToDec3Box(s.CodecPrivateData)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	init := segment.NewBaseInitSegment("audio", s.Lang, s.TimeScale, []string{"iso6", "piff", "mp4a"})
	err = init.Moov.Trak.SetEC3Descriptor(dec3Box)
	if err != nil {
		return nil, mp4.DecryptInfo{}, err
	}

	if s.KeyId != nil && s.Pssh != nil {
		decryptInfo, err := segment.AddPrEncryption(init, s.Key, s.KeyId, s.Pssh)
		return init, decryptInfo, err
	}

	return init, mp4.DecryptInfo{}, nil
}
