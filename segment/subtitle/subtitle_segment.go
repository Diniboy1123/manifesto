package subtitle

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/mp4"
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
func ProcessSubtitleSegment(input *bytes.Buffer, chunkId uint64) (output *bytes.Buffer, err error) {
	output = bytes.NewBuffer(nil)

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
		}

		// subtitle decryption is not supported
	}

	err = inMp4.Encode(output)
	if err != nil {
		return nil, fmt.Errorf("failed to encode segment: %v", err)
	}
	return output, nil
}
