package handlers

import (
	"bytes"
	"fmt"
	"io"
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

// SegmentHandler handles requests for segments of a stream.
// It retrieves the requested segment from the source URL, processes it, and
// returns the processed segment to the client.
//
// The handler expects the following URL parameters:
//   - channelId: The ID of the channel.
//   - qualityId: The ID of the quality level.
//   - time: The time of the segment.
//   - rest: The remaining part of the URL, which is the chunk name.
//
// The handler also expects the channel information to be present in the request context.
// If any of the required parameters are missing or invalid, it returns an error response.
//
// The handler supports different stream types (video, audio, text) and processes
// the segments accordingly. It also handles PR based segment decryption by extracting the key ID
// and PSSH data from the manifest. The processed segment is returned with the appropriate
// content type (video/mp4, audio/mp4, application/mp4).
func SegmentHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	rest := r.PathValue("rest")
	if rest == "" {
		http.Error(w, "No chunk specified", http.StatusBadRequest)
		return
	}

	timeStr := r.PathValue("time")
	if timeStr == "" {
		http.Error(w, "No time specified", http.StatusBadRequest)
		return
	}
	time, err := strconv.ParseUint(timeStr, 10, 64)
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
	case "text":
		// subtitle decryption isn't supported, so we don't need decryptInfo
	default:
		http.Error(w, "Unsupported stream type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating init segment: %v", err), http.StatusInternalServerError)
		return
	}

	// fetch channel.Url minus the last part of the path + rest
	chunkBase := channel.Url[:strings.LastIndex(channel.Url, "/")+1]
	chunkUrl := chunkBase + rest

	chunkReq, err := utils.DoRequest("GET", chunkUrl, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching chunk: %v", err), http.StatusInternalServerError)
		return
	}
	defer chunkReq.Body.Close()

	if chunkReq.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Error fetching chunk: %s", chunkReq.Status), http.StatusInternalServerError)
		return
	}

	chunkData, err := io.ReadAll(chunkReq.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading chunk data: %v", err), http.StatusInternalServerError)
		return
	}

	var output []byte
	switch streamIndex.Type {
	case "video":
		output, err = video.ProcessVideoSegment(bytes.NewBuffer(chunkData), decryptInfo, key, time)
	case "audio":
		output, err = audio.ProcessAudioSegment(bytes.NewBuffer(chunkData), decryptInfo, key, time)
	case "text":
		var firstSegmentDuration uint32
		if len(streamIndex.ChunkInfos) > 0 {
			firstSegmentDuration = uint32(streamIndex.ChunkInfos[0].Duration)
		}
		output, err = subtitle.ProcessSubtitleSegment(bytes.NewBuffer(chunkData), time, uint32(streamIndex.TimeScale), firstSegmentDuration)
	default:
		http.Error(w, "Unsupported stream type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error processing segment: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", streamIndex.GetMimeType())
	w.Header().Set("Content-Length", strconv.Itoa(len(output)))
	w.WriteHeader(http.StatusOK)

	w.Write(output)
}
