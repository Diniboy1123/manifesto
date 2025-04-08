package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Diniboy1123/manifesto/segment/audio"
	"github.com/Diniboy1123/manifesto/segment/subtitle"
	"github.com/Diniboy1123/manifesto/segment/video"
	"github.com/Diniboy1123/manifesto/transformers"
	"github.com/Eyevinn/mp4ff/mp4"
)

// InitHandler handles requests for the initialization segment of a stream.
// It retrieves the requested manifest from the source URL, and builds up an init segment for the requested
// quality level from scratch based on properties of the manifest.
//
// The handler expects the following URL parameters:
//   - channelId: The ID of the channel.
//   - qualityId: The ID of the quality level.
//
// The handler also expects the channel information to be present in the request context.
// If any of the required parameters are missing or invalid, it returns an error response.
//
// The handler supports different stream types (video, audio, text) and generates
// the initialization segments accordingly. It also takes care of potentially encrypted init segments
// (if no key is present, we return a segment for encrypted media) and strips encryption data if key is present.
//
// The handler also sets the Content-Disposition header to suggest a filename for the downloaded file.
// The filename is set to "init.mp4".
func InitHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	qualityId := r.PathValue("qualityId")

	// split from right, because we can have audio_deu_0 where we want audio_deu and 0
	lastUnderscore := strings.LastIndex(qualityId, "_")
	if lastUnderscore == -1 || lastUnderscore == len(qualityId)-1 {
		http.Error(w, "Invalid quality ID format", http.StatusBadRequest)
		return
	}

	streamIndexStr := qualityId[:lastUnderscore]
	qualityLevelIndexStr := qualityId[lastUnderscore+1:]
	qualityLevelIndex, err := strconv.Atoi(qualityLevelIndexStr)
	if err != nil {
		http.Error(w, "Invalid quality level index", http.StatusBadRequest)
		return
	}

	smoothStream, err := transformers.GetSmoothManifest(channel.Url)
	if err != nil {
		http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
		return
	}

	streamIndex, err := smoothStream.GetStreamIndexByNameOrType(streamIndexStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching stream index: %v", err), http.StatusInternalServerError)
		return
	}

	qualityLevel, err := streamIndex.GetQualityLevelByIndex(qualityLevelIndex)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching quality level: %v", err), http.StatusInternalServerError)
		return
	}

	var keyId []byte
	var key []byte
	var pssh []byte
	if smoothStream.Protection != nil {
		for _, key := range smoothStream.Protection {
			if strings.ToLower(key.SystemID) == mp4.UUIDPlayReady {
				pssh, err = base64.StdEncoding.DecodeString(key.CustomData)
				if err != nil {
					http.Error(w, fmt.Sprintf("Error decoding PSSH: %v", err), http.StatusInternalServerError)
					return
				}
				pssh = utils.TrimNullBytes(pssh)
				keyId, err = utils.ExtractPRKeyIdFromPssh(pssh)
				if err != nil {
					http.Error(w, fmt.Sprintf("Error extracting key ID: %v", err), http.StatusInternalServerError)
					return
				}
			}
		}

		if keyId == nil {
			http.Error(w, "No PlayReady key ID found", http.StatusInternalServerError)
			return
		}
		key, err = channel.GetKey(keyId)
		if err != nil {
			if err.Error() == "key not found" && channel.Keys != nil {
				http.Error(w, fmt.Sprintf("Error fetching key: %v", err), http.StatusInternalServerError)
				return
			}
		}
		if len(key) == 0 && channel.Keys != nil {
			http.Error(w, "Key not found", http.StatusInternalServerError)
			return
		}
	}

	baseSegment := segment.BaseInitSegment{
		TimeScale:        uint32(smoothStream.TimeScale),
		Lang:             streamIndex.Language,
		CodecPrivateData: qualityLevel.CodecPrivateData,
	}
	if keyId != nil {
		baseSegment.KeyId = keyId
		baseSegment.Key = key
		baseSegment.Pssh = pssh
	}

	var initSegment *mp4.InitSegment
	switch streamIndex.Type {
	case "video":
		avcInitSegment := video.AVCInitSegment{BaseInitSegment: baseSegment}
		initSegment, _, err = avcInitSegment.Generate()
	case "audio":
		if strings.ToLower(qualityLevel.FourCC) == "aacl" {
			aacInitSegment := audio.AACInitSegment{BaseInitSegment: baseSegment}
			initSegment, _, err = aacInitSegment.Generate()
		} else if strings.ToLower(qualityLevel.FourCC) == "ec-3" {
			de3InitSegment := audio.De3InitSegment{BaseInitSegment: baseSegment}
			initSegment, _, err = de3InitSegment.Generate()
		} else {
			http.Error(w, "Unsupported audio codec", http.StatusBadRequest)
			return
		}
	case "text":
		if strings.ToLower(qualityLevel.FourCC) == "ttml" {
			stppInitSegment := subtitle.STPPInitSegment{BaseInitSegment: baseSegment}
			initSegment, err = stppInitSegment.Generate()
		} else {
			http.Error(w, "Unsupported text codec", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "Unsupported stream type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating init segment: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", streamIndex.GetMimeType())
	w.Header().Set("Content-Disposition", "attachment; filename=init.mp4")
	w.WriteHeader(http.StatusOK)
	initSegment.Encode(w)
}
