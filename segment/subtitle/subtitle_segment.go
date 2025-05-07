package subtitle

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/Eyevinn/mp4ff/mp4"
)

const (
	DefaultSampleSizePresent      = 0x000010
	DefaultSampleDurationPresent  = 0x000008
	SampleDescriptionIndexPresent = 0x000002
)

// ProcessSubtitleSegment processes a subtitle segment, overrides the track ID,
// and adds a tfdt box if missing. It takes an input buffer containing the segment
// data and a chunk ID. It returns an output buffer containing the processed
// segment data and any error encountered during processing.
// The function handles fragmented MP4 files and ensures that the output is
// properly formatted for playback. It also handles the case where the input
// MP4 file is not fragmented, returning an error in that case.
//
// Note: Subtitle decryption is not supported in this implementation.
func ProcessSubtitleSegment(input *bytes.Buffer, chunkId uint64, timeScale uint32, segmentDuration uint32) ([]byte, error) {
	output := bytes.NewBuffer(nil)

	inMp4, err := mp4.DecodeFile(input)
	if err != nil {
		return nil, fmt.Errorf("failed to decode mp4 file: %v", err)
	}

	if !inMp4.IsFragmented() {
		return nil, fmt.Errorf("input mp4 file is not fragmented, this isn't supported")
	}

	for _, seg := range inMp4.Segments {
		for _, fragment := range seg.Fragments {
			fragment.Moof.Traf.Tfhd.TrackID = 1

			var hasTfdt bool
			for _, child := range fragment.Moof.Traf.Children {
				if child.Type() == "tfdt" {
					hasTfdt = true
					break
				}
			}

			// VLC has delayed audio when tfdt is missing
			// kinda hacky, because time isn't always equal to chunkId, but it works
			if !hasTfdt {
				fragment.Moof.Traf.AddChild(mp4.CreateTfdt(chunkId))
			}

			var hasSidx bool
			for _, child := range fragment.Children {
				if child.Type() == "sidx" {
					hasSidx = true
					break
				}
			}

			// Apparently the sidx box is required for ffmpeg to process subtitle streams without errors.
			// The timescale and duration values used here are not ideal, but if the remote end
			// does not provide these values, we rely on the values defined in the manifest.
			if !hasSidx && timeScale > 0 && segmentDuration > 0 {
				// Ensure the sidx box is added as the first child to avoid playback issues in some players.
				fragment.Children = append([]mp4.Box{
					&mp4.SidxBox{
						Version: 1,
						// ReferenceID corresponds to the hardcoded TrackID.
						ReferenceID: 1,
						Timescale:   timeScale,
						// EarliestPresentationTime is set to a value I observed in working samples.
						EarliestPresentationTime: 17443164950004000,
						FirstOffset:              0,
						SidxRefs: []mp4.SidxRef{
							{
								// ReferencedSize is set to 0 as a placeholder, which appears to work in practice (not ideal).
								ReferencedSize:     0,
								ReferenceType:      0,
								SubSegmentDuration: segmentDuration,
								// StartsWithSAP and SAPType are hardcoded based on observed manifest values.
								StartsWithSAP: 1,
								SAPType:       1,
								SAPDeltaTime:  0,
							},
						},
					},
				}, fragment.Children...)
			}

			// When we have a smooth streaming chunk, TTML subtitle timestamps are relative to the segment start time.
			// We need to ensure that those timestamps are absolute as MPEG-DASH requires absolute timestamps.
			// See: https://github.com/Dash-Industry-Forum/dash.js/blob/1c966e1f43094d6d9e7f80defef3f71c90ccdd73/src/streaming/text/TextSourceBuffer.js#L335
			segmentStartTime := float64(chunkId) / float64(timeScale)

			enhancedTTML, err := UpdateTTMLToAbsoluteTimestamps(string(fragment.Mdat.Data), segmentStartTime)
			if err != nil {
				log.Printf("failed to enhance TTML: %v", err)
				return nil, fmt.Errorf("failed to enhance TTML: %v", err)
			}

			fragment.Mdat.SetData([]byte(enhancedTTML))

			// When modifying the MDAT box, it is necessary to update the sample size. Most segments I encountered had
			// pre-populated sample definitions in the trun boxes, but certain players did not handle this well.
			// Setting the tfhd default sample size and default sample duration is generally better respected by players.
			// Additionally, an empty sample is added to the trun box to ensure the sample count is correct. It is important
			// not to remove the sample inside trun box entirely, as some players depend on its presence, even if it has no properties set.
			//
			// NOTE: This approach does not support cases with multiple mdat chunks, but such cases have not been observed by me for subtitles.
			fragment.Moof.Traf.Tfhd.DefaultSampleSize = uint32(len(enhancedTTML))
			fragment.Moof.Traf.Tfhd.DefaultSampleDuration = segmentDuration

			fragment.Moof.Traf.Tfhd.Flags |= DefaultSampleSizePresent |
				DefaultSampleDurationPresent

			for _, truns := range fragment.Moof.Traf.Truns {
				truns.Flags &^= mp4.TrunFirstSampleFlagsPresentFlag |
					mp4.TrunSampleDurationPresentFlag |
					mp4.TrunSampleSizePresentFlag |
					mp4.TrunSampleFlagsPresentFlag |
					mp4.TrunSampleCompositionTimeOffsetPresentFlag
				truns.Samples = []mp4.Sample{}
				truns.AddSample(mp4.Sample{})
			}
		}

		// subtitle decryption is not supported
	}

	err = inMp4.Encode(output)
	if err != nil {
		return nil, fmt.Errorf("failed to encode segment: %v", err)
	}

	return output.Bytes(), nil
}

// UpdateTTMLToAbsoluteTimestamps updates relative TTML timestamps to absolute ones for smooth streaming manifests.
// It parses the TTML XML, adjusts the 'begin' and 'end' attributes of <p> elements by adding the segment's start time in seconds,
// and returns the modified TTML as a string. Returns an error if XML parsing fails.
func UpdateTTMLToAbsoluteTimestamps(input string, segmentStartSeconds float64) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(input))
	var output bytes.Buffer

	// This is a hardcore approach, because go's built-in XML decoder doesn't support namespaces.
	// Therefore instead of parsing the entire XML as a struct, we look for the <p> elements and process them manually.
	for {
		tok, err := decoder.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("xml decode error: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				output.WriteString("<" + t.Name.Local)
				for _, attr := range t.Attr {
					val := attr.Value
					if attr.Name.Local == "begin" || attr.Name.Local == "end" {
						if seconds, err := parseTTMLTime(val); err == nil {
							val = formatTTMLTime(seconds + segmentStartSeconds)
						}
					}
					writeAttr(&output, attr.Name, val)
				}
				output.WriteString(">")
			} else {
				writeStartElement(&output, t)
			}
		case xml.EndElement:
			output.WriteString(fmt.Sprintf("</%s>", t.Name.Local))
		case xml.CharData:
			output.WriteString(html.EscapeString(string(t)))
		case xml.Comment:
			output.WriteString("<!--" + string(t) + "-->")
		case xml.ProcInst:
			output.WriteString(fmt.Sprintf("<?%s %s?>", t.Target, string(t.Inst)))
		case xml.Directive:
			output.WriteString(fmt.Sprintf("<!%s>", string(t)))
		}
	}
	outputString := output.String()
	return outputString, nil
}

// parseTTMLTime parses a TTML time string (e.g., "00:01:02.345" or seconds as float) and returns the time in seconds.
func parseTTMLTime(value string) (float64, error) {
	if strings.Contains(value, ":") {
		var h, m int
		var s float64
		_, err := fmt.Sscanf(value, "%d:%d:%f", &h, &m, &s)
		if err != nil {
			return 0, err
		}
		return float64(h)*3600 + float64(m)*60 + s, nil
	}
	return strconv.ParseFloat(value, 64)
}

// formatTTMLTime formats a float64 number of seconds as a TTML time string (e.g., "00:01:02.345").
func formatTTMLTime(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := seconds - float64(h*3600+m*60)
	return fmt.Sprintf("%02d:%02d:%06.3f", h, m, s)
}

// writeAttr writes an XML attribute to the buffer in the correct format.
func writeAttr(buf *bytes.Buffer, name xml.Name, val string) {
	if name.Space != "" {
		buf.WriteString(fmt.Sprintf(" %s:%s=\"%s\"", name.Space, name.Local, val))
	} else {
		buf.WriteString(fmt.Sprintf(" %s=\"%s\"", name.Local, val))
	}
}

// writeStartElement writes an XML start element and its attributes to the buffer.
func writeStartElement(buf *bytes.Buffer, el xml.StartElement) {
	buf.WriteString("<" + el.Name.Local)
	for _, attr := range el.Attr {
		writeAttr(buf, attr.Name, attr.Value)
	}
	buf.WriteString(">")
}
