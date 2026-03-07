package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Diniboy1123/manifesto/segment/audio"
	"github.com/Diniboy1123/manifesto/segment/ts"
	"github.com/Diniboy1123/manifesto/segment/video"
	"github.com/Diniboy1123/manifesto/transformers"
	"github.com/Eyevinn/mp4ff/mp4"
)

// HlsSegmentHandler handles requests for HLS MPEG-TS segments.
// It fetches the corresponding smooth streaming chunk, decrypts if needed,
// extracts the raw media samples, and repackages them into MPEG-TS format.
//
// The handler expects the following URL parameters:
//   - channelId: The ID of the channel.
//   - qualityId: The ID of the quality level (e.g., "video_0", "audio_eng_0").
//   - time: The segment time in the manifest's timescale.
//
// The handler reconstructs the origin chunk URL from the manifest's URL template,
// so it does not require the chunk path in the URL (unlike the DASH segment handler).
func HlsSegmentHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	timeStr := r.PathValue("time")
	if timeStr == "" {
		http.Error(w, "No time specified", http.StatusBadRequest)
		return
	}
	segmentTime, err := strconv.ParseUint(timeStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid time format", http.StatusBadRequest)
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
		log.Printf("Error fetching manifest: %v", err)
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
	var decryptInfo mp4.DecryptInfo
	switch streamIndex.Type {
	case "video":
		avcInitSegment := video.AVCInitSegment{BaseInitSegment: baseSegment}
		_, decryptInfo, err = avcInitSegment.Generate()
	case "audio":
		switch qualityLevel.FourCC {
		case "AACL":
			aacInitSegment := audio.AACInitSegment{BaseInitSegment: baseSegment}
			_, decryptInfo, err = aacInitSegment.Generate()
		case "EC-3":
			de3InitSegment := audio.De3InitSegment{BaseInitSegment: baseSegment}
			_, decryptInfo, err = de3InitSegment.Generate()
		default:
			http.Error(w, "Unsupported audio codec", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "Unsupported stream type for HLS TS", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating init segment: %v", err), http.StatusInternalServerError)
		return
	}
	initGenTook := time.Since(initGenStartTime)

	// Construct the chunk URL from the manifest's URL template
	chunkBase := channel.Url[:strings.LastIndex(channel.Url, "/")+1]
	chunkPath := strings.NewReplacer(
		"{bitrate}", strconv.FormatUint(qualityLevel.Bitrate, 10),
		"{start time}", strconv.FormatUint(segmentTime, 10),
	).Replace(streamIndex.Url)
	chunkUrl := chunkBase + chunkPath

	chunkFetchStartTime := time.Now()
	chunkResp, err := utils.DoRequest("GET", chunkUrl, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching chunk: %v", err), http.StatusInternalServerError)
		return
	}
	defer chunkResp.Body.Close()
	chunkFetchTook := time.Since(chunkFetchStartTime)

	if chunkResp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Error fetching chunk: %s", chunkResp.Status), http.StatusInternalServerError)
		return
	}

	chunkData, err := io.ReadAll(chunkResp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading chunk data: %v", err), http.StatusInternalServerError)
		return
	}

	segmentProcessStartTime := time.Now()

	w.Header().Set("Content-Type", "video/mp2t")

	switch streamIndex.Type {
	case "video":
		spsNALUs, ppsNALUs, err := video.CodecPrivateDataToSPSPPS(qualityLevel.CodecPrivateData)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error parsing codec data: %v", err), http.StatusInternalServerError)
			return
		}
		err = ts.MuxVideoSegmentToTS(r.Context(), w, chunkData, decryptInfo, key, spsNALUs, ppsNALUs, uint32(smoothStream.TimeScale), segmentTime)
	case "audio":
		switch qualityLevel.FourCC {
		case "AACL":
			audioConfig, err := audio.CodecPrivateDataToAudioSpecificConfig(qualityLevel.CodecPrivateData)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error parsing audio config: %v", err), http.StatusInternalServerError)
				return
			}
			err = ts.MuxAudioAACSegmentToTS(r.Context(), w, chunkData, decryptInfo, key,
				audioConfig.ObjectType, audioConfig.SamplingFrequency, audioConfig.ChannelConfiguration,
				uint32(smoothStream.TimeScale), segmentTime)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error muxing audio to TS: %v", err), http.StatusInternalServerError)
				return
			}
		case "EC-3":
			err = ts.MuxAudioEAC3SegmentToTS(r.Context(), w, chunkData, decryptInfo, key,
				uint32(smoothStream.TimeScale), segmentTime)
		default:
			http.Error(w, "Unsupported audio codec for HLS TS", http.StatusBadRequest)
			return
		}
	}

	if err != nil {
		log.Printf("Error muxing segment to TS: %v", err)
		return
	}
	segmentProcessTook := time.Since(segmentProcessStartTime)

	reqStartTime := r.Context().Value("reqStartTime").(time.Time)
	reqTook := time.Since(reqStartTime)

	w.Header().Set("Server-Timing", fmt.Sprintf(
		"manifest-fetch;dur=%.3f,init-gen;dur=%.3f,chunk-fetch;dur=%.3f,segment-process;dur=%.3f,total;dur=%.3f",
		manifestFetchTook.Seconds()*1000,
		initGenTook.Seconds()*1000,
		chunkFetchTook.Seconds()*1000,
		segmentProcessTook.Seconds()*1000,
		reqTook.Seconds()*1000,
	))
}
