package handlers

import (
	"bytes"
	"encoding/base64"
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

	var decryptInfo mp4.DecryptInfo
	switch streamIndex.Type {
	case "video":
		avcInitSegment := video.AVCInitSegment{BaseInitSegment: baseSegment}
		_, decryptInfo, err = avcInitSegment.Generate()
	case "audio":
		if qualityLevel.FourCC == "AACL" {
			aacInitSegment := audio.AACInitSegment{BaseInitSegment: baseSegment}
			_, decryptInfo, err = aacInitSegment.Generate()
		} else if qualityLevel.FourCC == "EC-3" {
			de3InitSegment := audio.De3InitSegment{BaseInitSegment: baseSegment}
			_, decryptInfo, err = de3InitSegment.Generate()
		} else {
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

	var output *bytes.Buffer
	switch streamIndex.Type {
	case "video":
		output, err = video.ProcessVideoSegment(bytes.NewBuffer(chunkData), decryptInfo, key, time)
	case "audio":
		output, err = audio.ProcessAudioSegment(bytes.NewBuffer(chunkData), decryptInfo, key, time)
	case "text":
		output, err = subtitle.ProcessSubtitleSegment(bytes.NewBuffer(chunkData), time)
	default:
		http.Error(w, "Unsupported stream type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error processing segment: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", streamIndex.GetMimeType())

	_, err = io.Copy(w, output)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing chunk data: %v", err), http.StatusInternalServerError)
		return
	}
}
