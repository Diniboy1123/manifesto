package transformers

import (
	"fmt"
	"math"
	"strings"

	"github.com/Diniboy1123/manifesto/models"
	"github.com/Diniboy1123/manifesto/segment/video"
	"github.com/Eyevinn/mp4ff/avc"
)

// SmoothToHlsMasterPlaylist generates an HLS master playlist (M3U8) from a SmoothStream manifest.
// It takes the ISM manifest and a boolean indicating if keys are present.
// It returns the generated HLS master playlist as bytes.
//
// The function generates EXT-X-MEDIA tags for audio renditions and
// EXT-X-STREAM-INF tags for video variants. Segments are served as MPEG-TS.
func SmoothToHlsMasterPlaylist(ismManifest *models.SmoothStream, hasKeys bool) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n")
	sb.WriteString("\n")

	var audioGroupId string
	var defaultAudioCodec string
	hasDefaultAudio := false

	// First pass: create EXT-X-MEDIA tags for audio streams
	for _, streamIndex := range ismManifest.StreamIndexes {
		if streamIndex.Type != "audio" {
			continue
		}

		var streamIndexName string
		if streamIndex.Name != "" {
			streamIndexName = streamIndex.Name
		} else {
			streamIndexName = streamIndex.Type
		}

		audioGroupId = "audio"

		for _, ql := range streamIndex.QualityLevels {
			id := fmt.Sprintf("%s_%d", streamIndexName, ql.Index)
			isDefault := !hasDefaultAudio

			channels := ql.Channels
			if channels <= 0 {
				channels = 2
			}

			sb.WriteString(fmt.Sprintf(
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"%s\",NAME=\"%s\",LANGUAGE=\"%s\",DEFAULT=%s,AUTOSELECT=YES,CHANNELS=\"%d\",URI=\"%s/playlist.m3u8\"\n",
				audioGroupId,
				id,
				streamIndex.Language,
				boolToYesNo(isDefault),
				channels,
				id,
			))

			if defaultAudioCodec == "" {
				switch ql.FourCC {
				case "EC-3":
					defaultAudioCodec = "ec-3"
				default:
					defaultAudioCodec = "mp4a.40.2"
				}
			}
			hasDefaultAudio = true
		}
	}

	if audioGroupId != "" {
		sb.WriteString("\n")
	}

	// Second pass: create EXT-X-STREAM-INF for video streams
	for _, streamIndex := range ismManifest.StreamIndexes {
		if streamIndex.Type != "video" {
			continue
		}

		var streamIndexName string
		if streamIndex.Name != "" {
			streamIndexName = streamIndex.Name
		} else {
			streamIndexName = streamIndex.Type
		}

		for _, ql := range streamIndex.QualityLevels {
			id := fmt.Sprintf("%s_%d", streamIndexName, ql.Index)

			var codecStr string
			if ql.CodecPrivateData != "" {
				spsNALUs, _, err := video.CodecPrivateDataToSPSPPS(ql.CodecPrivateData)
				if err == nil && len(spsNALUs) > 0 {
					sps, err := avc.ParseSPSNALUnit(spsNALUs[0], false)
					if err == nil {
						codecStr = avc.CodecString("avc1", sps)
					}
				}
			}

			codecs := codecStr
			if defaultAudioCodec != "" {
				if codecs != "" {
					codecs += "," + defaultAudioCodec
				} else {
					codecs = defaultAudioCodec
				}
			}

			var extras []string
			if audioGroupId != "" {
				extras = append(extras, fmt.Sprintf("AUDIO=\"%s\"", audioGroupId))
			}

			extraStr := ""
			if len(extras) > 0 {
				extraStr = "," + strings.Join(extras, ",")
			}

			codecsAttr := ""
			if codecs != "" {
				codecsAttr = fmt.Sprintf(",CODECS=\"%s\"", codecs)
			}

			resolutionAttr := ""
			if ql.MaxWidth > 0 && ql.MaxHeight > 0 {
				resolutionAttr = fmt.Sprintf(",RESOLUTION=%dx%d", ql.MaxWidth, ql.MaxHeight)
			}

			sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d%s%s%s\n",
				ql.Bitrate,
				resolutionAttr,
				codecsAttr,
				extraStr,
			))
			sb.WriteString(fmt.Sprintf("%s/playlist.m3u8\n", id))
		}
	}

	return []byte(sb.String()), nil
}

// SmoothToHlsMediaPlaylist generates an HLS media playlist (M3U8) for a specific quality level
// of a stream index. It uses MPEG-TS segments with URLs pointing to the HLS TS segment handler.
func SmoothToHlsMediaPlaylist(ismManifest *models.SmoothStream, streamIndexName string, qualityLevelIndex int) ([]byte, error) {
	streamIndex, err := ismManifest.GetStreamIndexByNameOrType(streamIndexName)
	if err != nil {
		return nil, err
	}

	// Validate that the quality level exists
	if _, err := streamIndex.GetQualityLevelByIndex(qualityLevelIndex); err != nil {
		return nil, err
	}

	timeScale := ismManifest.TimeScale
	if timeScale == 0 {
		timeScale = 10000000
	}

	var sb strings.Builder

	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n")

	// Calculate target duration (max segment duration in seconds, rounded up)
	var maxDuration float64
	for _, chunk := range streamIndex.ChunkInfos {
		dur := float64(chunk.Duration) / float64(timeScale)
		if dur > maxDuration {
			maxDuration = dur
		}
	}
	targetDuration := int(math.Ceil(maxDuration))
	if targetDuration < 1 {
		targetDuration = 6
	}

	sb.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))
	sb.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	if !ismManifest.IsLive {
		sb.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	}

	sb.WriteString("\n")

	// Generate segment entries
	currentTime := uint64(0)
	for i, chunk := range streamIndex.ChunkInfos {
		if chunk.StartTime > 0 {
			currentTime = chunk.StartTime
		} else if i == 0 {
			currentTime = 0
		}

		duration := float64(chunk.Duration) / float64(timeScale)

		sb.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", duration))
		sb.WriteString(fmt.Sprintf("%d/seg.ts\n", currentTime))

		currentTime += chunk.Duration
	}

	if !ismManifest.IsLive {
		sb.WriteString("#EXT-X-ENDLIST\n")
	}

	return []byte(sb.String()), nil
}

func boolToYesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}
