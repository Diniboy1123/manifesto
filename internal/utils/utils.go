package utils

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"unicode/utf16"

	"github.com/Diniboy1123/manifesto/models"
	"github.com/Eyevinn/mp4ff/mp4"
)

// PlayReadyRegexp is a regular expression to extract the KID from PlayReady PSSH data.
// It matches the pattern <KID>...</KID> and captures the base64-encoded KID value.
// The KID is a 16-byte value used for PlayReady DRM.
var PlayReadyRegexp = regexp.MustCompile(`<KID>([a-zA-Z0-9+/=]+)</KID>`)

// ExtractPRKeyIdFromPssh extracts the PlayReady key ID from the PSSH data.
// It decodes the PSSH data from UTF-16 and uses a regular expression to find the KID.
// The KID is then base64-decoded and returned as a byte slice.
// If the KID is not found or if there is an error during decoding, it returns an error.
//
// The function expects the PSSH data to be in the format defined by PlayReady.
func ExtractPRKeyIdFromPssh(data []byte) ([]byte, error) {
	shorts := make([]uint16, (len(data)-10)/2)
	for i := range shorts {
		shorts[i] = uint16(data[10+2*i]) | uint16(data[11+2*i])<<8
	}
	decoded := utf16.Decode(shorts)
	match := PlayReadyRegexp.FindStringSubmatch(string(decoded))
	if len(match) < 2 {
		return nil, nil
	}

	keyBytes, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil || len(keyBytes) != 16 {
		return nil, err
	}

	uuid := []byte{
		keyBytes[3], keyBytes[2], keyBytes[1], keyBytes[0],
		keyBytes[5], keyBytes[4],
		keyBytes[7], keyBytes[6],
		keyBytes[8], keyBytes[9],
		keyBytes[10], keyBytes[11], keyBytes[12], keyBytes[13], keyBytes[14], keyBytes[15],
	}
	return uuid, nil
}

// TrimNullBytes trims null bytes from the end of the given byte slice.
//
// Some providers may add numerous null bytes to PSSH data which leads to extra memory usage.
// This function removes those null bytes to optimize memory usage.
func TrimNullBytes(data []byte) []byte {
	return bytes.TrimRight(data, "\x00")
}

// GeneratePsshData generates PSSH data for PlayReady DRM.
// It takes the PlayReady protection data, decodes the custom data from base64,
// and packs it to a newly created PSSH box.
// The PSSH box is then encoded to a byte slice and returned as a base64-encoded string.
//
// If the PlayReady protection data is nil, it returns an empty string and no error.
func GeneratePsshData(playreadyProtectionData *models.SmoothProtectionHeader) (string, error) {
	if playreadyProtectionData == nil {
		return "", nil
	}

	customDataDecoded, err := base64.StdEncoding.DecodeString(playreadyProtectionData.CustomData)
	if err != nil {
		return "", err
	}

	uuid, err := mp4.NewUUIDFromString(mp4.UUIDPlayReady)
	if err != nil {
		return "", err
	}

	psshBox := &mp4.PsshBox{
		Version:  0,
		Flags:    0,
		SystemID: uuid,
		Data:     customDataDecoded,
	}

	psshDataBytes := bytes.NewBuffer(nil)
	if err := psshBox.Encode(psshDataBytes); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(psshDataBytes.Bytes()), nil
}
