package video

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/mp4"
)

// ProcessVideoSegment processes a video segment, overrides the track ID,
// and adds a tfdt box if missing. It takes an input buffer containing the segment if a key is provided.
// It takes an input buffer containing the segment data, decrypt information,
// a key for decryption, and a chunk ID. It returns an output buffer containing
// the processed segment data and any error encountered during processing.
// The function handles fragmented MP4 files and ensures that the output is
// properly formatted for playback. It also handles the case where the input
// MP4 file is not fragmented, returning an error in that case.
//
// The function also removes the sdtp box if present, as it is not needed for MPEG-DASH.
// The tfdt box is added if it is missing, as some players require it for proper track synchronization.
func ProcessVideoSegment(input *bytes.Buffer, decryptInfo mp4.DecryptInfo, key []byte, chunkId uint64) ([]byte, error) {
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
			// required for proper decryption
			fragment.Moof.Traf.Tfhd.TrackID = 1
			// some providers have broken values
			// which makes mp4ff panic
			fragment.Moof.Traf.Trun.DataOffset = 0

			// check if Moof.Traf.Children has SdtpBox
			// if so, remove it
			// according to ISO_IEC_14496-12_2015 its useful for seeking
			// https://wiki.gpac.io/MP4Box/mp4box-dash-opts/#options this states that its smooth like
			// stream works fine without it
			var hasTfdt bool
			for i, child := range fragment.Moof.Traf.Children {
				if child.Type() == "sdtp" {
					fragment.Moof.Traf.Children = append(fragment.Moof.Traf.Children[:i], fragment.Moof.Traf.Children[i+1:]...)
				}
				if child.Type() == "tfdt" {
					hasTfdt = true
				}
			}

			// VLC has delayed audio when tfdt is missing
			// kinda hacky, because time isn't always equal to chunkId, but it works
			if !hasTfdt {
				fragment.Moof.Traf.AddChild(mp4.CreateTfdt(chunkId))
			}
		}

		if key != nil {
			err = mp4.DecryptSegment(seg, decryptInfo, key)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt segment: %v", err)
			}
		}
	}

	err = inMp4.Encode(output)
	if err != nil {
		return nil, fmt.Errorf("failed to encode decrypted segment: %v", err)
	}

	return output.Bytes(), nil
}
