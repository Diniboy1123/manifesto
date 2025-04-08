package segment

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/mp4"
)

// BaseInitSegment represents the base initialization segment for MP4 files.
type BaseInitSegment struct {
	// TimeScale is the time scale for the segment.
	TimeScale uint32
	// Lang is the language of the segment. You may set to "und" if not specified.
	Lang string
	// CodecPrivateData is the codec private data in hex string format.
	CodecPrivateData string
	// KeyId is the key ID for encryption.
	KeyId []byte
	// Key is the encryption key.
	Key []byte
	// Pssh is the PSSH data for encryption.
	Pssh []byte
}

// AddPrEncryption adds encryption information to the initialization segment.
// It initializes the protection system and returns the decryption information.
// If the key is nil, it returns an empty DecryptInfo.
//
// If an error occurs during the process, it returns the error.
func AddPrEncryption(init *mp4.InitSegment, key, keyId, pssh []byte) (mp4.DecryptInfo, error) {
	uuid, err := mp4.NewUUIDFromString(mp4.UUIDPlayReady)
	if err != nil {
		return mp4.DecryptInfo{}, fmt.Errorf("failed to parse UUID: %v", err)
	}
	psshBox := mp4.PsshBox{
		SystemID: uuid,
		Data:     pssh,
	}

	_, err = mp4.InitProtect(init, key, nil, "cenc", keyId, []*mp4.PsshBox{&psshBox})
	if err != nil {
		return mp4.DecryptInfo{}, fmt.Errorf("failed to initialize protection: %v", err)
	}

	if key == nil {
		return mp4.DecryptInfo{}, nil
	}

	decryptInfo, err := mp4.DecryptInit(init)
	if err != nil {
		return mp4.DecryptInfo{}, fmt.Errorf("failed to decrypt init segment: %v", err)
	}

	return decryptInfo, nil
}

// NewBaseInitSegment creates a new base initialization segment with the specified parameters.
// It initializes the segment with the given track type, language, time scale, and ftyp brands.
// It returns a pointer to the created InitSegment object.
//
// The function sets up the necessary boxes and tracks for the initialization segment.
// It adds a ftyp box with the specified brands and creates a moov box with the necessary
// components. The function also adds an empty track with the specified time scale, track type,
// and language.
func NewBaseInitSegment(trackType, lang string, timeScale uint32, ftypBrands []string) *mp4.InitSegment {
	init := mp4.NewMP4Init()
	init.AddChild(mp4.NewFtyp("dash", 0, ftypBrands))
	moov := mp4.NewMoovBox()
	init.AddChild(moov)
	moov.AddChild(mp4.CreateMvhd())
	moov.AddChild(mp4.NewMvexBox())
	init.AddEmptyTrack(timeScale, trackType, lang)
	return init
}
