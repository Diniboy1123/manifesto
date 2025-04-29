package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	manifestFetchStartTime := time.Now()
	smoothStream, err := transformers.GetSmoothManifest(channel.Url)
	if err != nil {
		http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
		return
	}
	manifestFetchTook := time.Since(manifestFetchStartTime)

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

	var keyId, key, pssh []byte
	if smoothStream.Protection != nil {
		keyId, key, pssh, err = utils.ExtractKeyInfo(smoothStream.Protection, channel)
		if err != nil {
			http.Error(w, fmt.Sprintf("DRM Error: %v", err), http.StatusInternalServerError)
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

	initGenStartTime := time.Now()
	var initSegment *mp4.InitSegment
	switch streamIndex.Type {
	case "video":
		avcInitSegment := video.AVCInitSegment{BaseInitSegment: baseSegment}
		initSegment, _, err = avcInitSegment.Generate()
	case "audio":
		switch strings.ToLower(qualityLevel.FourCC) {
		case "aacl":
			aacInitSegment := audio.AACInitSegment{BaseInitSegment: baseSegment}
			initSegment, _, err = aacInitSegment.Generate()
		case "ec-3":
			de3InitSegment := audio.De3InitSegment{BaseInitSegment: baseSegment}
			initSegment, _, err = de3InitSegment.Generate()
		default:
			http.Error(w, "Unsupported audio codec", http.StatusBadRequest)
			return
		}
	case "text":
		switch strings.ToLower(qualityLevel.FourCC) {
		case "ttml":
			stppInitSegment := subtitle.STPPInitSegment{BaseInitSegment: baseSegment}
			initSegment, err = stppInitSegment.Generate()
		default:
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
	initGenTook := time.Since(initGenStartTime)

	reqStartTime := r.Context().Value("reqStartTime").(time.Time)
	reqTook := time.Since(reqStartTime)

	w.Header().Set("Content-Type", streamIndex.GetMimeType())
	w.Header().Set("Content-Disposition", "attachment; filename=init.mp4")
	w.Header().Set("Server-Timing", fmt.Sprintf(
		"manifest-fetch;dur=%.3f,init-gen;dur=%.3f,total;dur=%.3f",
		manifestFetchTook.Seconds()*1000,
		initGenTook.Seconds()*1000,
		reqTook.Seconds()*1000,
	))
	w.WriteHeader(http.StatusOK)
	initSegment.Encode(w)
}
