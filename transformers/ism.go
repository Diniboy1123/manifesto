package transformers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
	"github.com/Diniboy1123/manifesto/models"
	"github.com/Diniboy1123/manifesto/segment/video"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/unki2aut/go-xsd-types"
)

// GetSmoothManifest requests the ISM manifest from the given URL and parses it into a SmoothStream object
//
// If the request fails, it returns an error.
func GetSmoothManifest(url string) (*models.SmoothStream, error) {
	content, err := utils.DoRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer content.Body.Close()

	ismManifest, err := models.NewSmoothStream(content.Body)
	if err != nil {
		return nil, err
	}

	return ismManifest, nil
}

// SmoothToDashManifest converts a SmoothStream manifest to a DASH manifest.
// It takes the ISM manifest, a boolean indicating if keys are present, and a boolean indicating if subtitles are allowed in the output.
// It returns the generated DASH manifest and any error encountered during the conversion process.
//
// The function processes the ISM manifest, extracting relevant information such as adaptation sets, segment templates,
// and representations. It handles different stream types (video, audio, text) and sets appropriate attributes for each representation.
// It also manages content protection information, including PlayReady protection data and PSSH data.
// The generated DASH manifest is structured according to the DASH-IF specifications, including necessary attributes such as
// availability start time, publish time, and period information.
// The function also sets the broadcast type based on whether the manifest is live or static.
func SmoothToDashManifest(ismManifest *models.SmoothStream, hasKeys, allowSubs bool, channel config.Channel) (*models.MPD, error) {
	playreadyProtectionData := ismManifest.GetProtectionHeaderForSystemId(mp4.UUIDPlayReady)

	var psshData string
	var err error
	if playreadyProtectionData != nil {
		psshData, err = utils.GeneratePsshData(playreadyProtectionData)
		if err != nil {
			return nil, err
		}
	}

	var adaptationSets []*models.AdaptationSet
	// streamindex to segmenttemplate
	for index, streamIndex := range ismManifest.StreamIndexes {
		var segmentTimelineSs []models.SegmentTimelineS
		for chunk, info := range streamIndex.ChunkInfos {
			segment := models.SegmentTimelineS{
				D: info.Duration,
			}

			if chunk == 0 {
				segment.T = info.StartTime
			}

			segmentTimelineSs = append(segmentTimelineSs, segment)
		}

		var streamIndexName string
		if streamIndex.Name != "" {
			streamIndexName = streamIndex.Name
		} else {
			streamIndexName = streamIndex.Type
		}

		segmentTemplate := &models.SegmentTemplate{
			Timescale:       ismManifest.TimeScale,
			Media:           "$RepresentationID$/$Time$/" + convertSmoothToMpdTag(streamIndex.Url),
			Initialization:  "$RepresentationID$/init.mp4",
			SegmentTimeline: &models.SegmentTimeline{S: segmentTimelineSs},
		}

		// qualityLevel to representation
		var representations []*models.Representation
		audioChannels := 2 // default to stereo
		for _, qualityLevel := range streamIndex.QualityLevels {
			id := fmt.Sprintf("%s_%d", streamIndexName, qualityLevel.Index)
			representation := models.Representation{
				ID:        id,
				Bandwidth: qualityLevel.Bitrate,
			}

			switch streamIndex.Type {
			case "video":
				// video has width, height and scantype
				representation.Width = qualityLevel.MaxWidth
				representation.Height = qualityLevel.MaxHeight

				if qualityLevel.CodecPrivateData == "" {
					return nil, fmt.Errorf("CodecPrivateData is empty for quality level %d", qualityLevel.Index)
				}

				spsNALUs, _, err := video.CodecPrivateDataToSPSPPS(qualityLevel.CodecPrivateData)
				if err != nil {
					return nil, fmt.Errorf("failed to parse CodecPrivateData for quality level %d: %w", qualityLevel.Index, err)
				}

				sps, err := avc.ParseSPSNALUnit(spsNALUs[0], false)
				if err != nil {
					return nil, err
				}

				representation.Codecs = avc.CodecString("avc1", sps)
				// hardcoded for now
				representation.ScanType = "progressive"
			case "audio":
				// audio has AudioSamplingRate

				if qualityLevel.Channels > 0 {
					audioChannels = qualityLevel.Channels
				}

				representation.AudioSamplingRate = strconv.FormatInt(qualityLevel.SamplingRate, 10)
				switch qualityLevel.FourCC {
				case "EC-3":
					representation.Codecs = "ec-3"
				default:
					representation.Codecs = "mp4a.40.2"
				}
			case "text":
				// TODO: don't hardcode
				representation.Codecs = "stpp"
			}

			representations = append(representations, &representation)
		}

		adaptationSet := &models.AdaptationSet{
			MimeType:         streamIndex.GetMimeType(),
			ContentType:      streamIndex.Type,
			ID:               strconv.FormatInt(int64(index), 10),
			SegmentAlignment: models.ConditionalUint{B: new(bool)},
			Lang:             streamIndex.Language,
			StartWithSAP:     models.ConditionalUint{U: new(uint64)},
			SegmentTemplate:  segmentTemplate,
			Representations:  representations,
		}

		*adaptationSet.SegmentAlignment.B = true
		*adaptationSet.StartWithSAP.U = 1

		switch streamIndex.Type {
		case "video":
			if !hasKeys && playreadyProtectionData != nil {
				adaptationSet.ContentProtections = []models.Descriptor{
					{
						SchemeIDURI: "urn:uuid:" + strings.ToLower(playreadyProtectionData.SystemID),
						Value:       "MSPR 2.0",
						Pro: &models.Pro{
							XMLNS: "urn:microsoft:playready",
							Data:  playreadyProtectionData.CustomData,
						},
						Pssh: &models.Pssh{
							XMLNS: "urn:mpeg:cenc:2013",
							Data:  psshData,
						},
					},
				}
			}
		case "audio":
			adaptationSet.AudioChannelConfiguration = &models.AudioChannelConfiguration{
				SchemeIdUri: "urn:mpeg:dash:23003:3:audio_channel_configuration:2011",
				Value:       fmt.Sprint(audioChannels),
			}
			if !hasKeys && playreadyProtectionData != nil {
				adaptationSet.ContentProtections = []models.Descriptor{
					{
						SchemeIDURI: "urn:uuid:" + strings.ToLower(playreadyProtectionData.SystemID),
						Value:       "MSPR 2.0",
						Pro: &models.Pro{
							XMLNS: "urn:microsoft:playready",
							Data:  playreadyProtectionData.CustomData,
						},
						Pssh: &models.Pssh{
							XMLNS: "urn:mpeg:cenc:2013",
							Data:  psshData,
						},
					},
				}
			}
		case "text":
			if !allowSubs {
				continue
			}
		}

		adaptationSets = append(adaptationSets, adaptationSet)
	}

	period := &models.Period{
		Start:          &xsd.Duration{Seconds: 0},
		ID:             "0",
		AdaptationSets: adaptationSets,
	}

	var broadcastType string
	if ismManifest.IsLive {
		broadcastType = "dynamic"
	} else {
		broadcastType = "static"
	}

	dashManifest := &models.MPD{
		XMLNS:    "urn:mpeg:dash:schema:mpd:2011",
		Type:     broadcastType,
		Profiles: "urn:mpeg:dash:profile:isoff-live:2011",
		// TODO: don't hardcode
		MinBufferTime:         &xsd.Duration{Seconds: 2},
		AvailabilityStartTime: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05Z"),
		PublishTime:           time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Period:                []*models.Period{period},
		UTCTiming: &models.UTCTiming{
			SchemeIdUri: "urn:mpeg:dash:utc:direct:2014",
			Value:       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		},
		ProgramInformation: &models.ProgramInformation{
			Title:     channel.Name,
			Copyright: "Served by manifesto",
		},
	}

	if !ismManifest.IsLive && ismManifest.Duration > 0 {
		dashManifest.MediaPresentationDuration = &xsd.Duration{Seconds: int64(ismManifest.Duration / 10000000)}
	}

	if ismManifest.IsLive {
		dashManifest.MinimumUpdatePeriod = &xsd.Duration{Seconds: 2}

		if ismManifest.DVRWindowLength > 0 {
			dashManifest.TimeShiftBufferDepth = &xsd.Duration{Seconds: ismManifest.DVRWindowLength / 10000000}
		}
	}

	if !hasKeys && playreadyProtectionData != nil {
		dashManifest.XMLNSPlayReady = "urn:microsoft:playready"
		dashManifest.XMLNSCommonEncryption = "urn:mpeg:cenc:2013"
	}

	return dashManifest, nil
}

func convertSmoothToMpdTag(path string) string {
	replacer := strings.NewReplacer(
		"{bitrate}", "$Bandwidth$",
		"{start time}", "$Time$",
	)
	return replacer.Replace(path)
}
