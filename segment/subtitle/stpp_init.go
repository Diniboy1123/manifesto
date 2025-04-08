package subtitle

import (
	"github.com/Diniboy1123/manifesto/segment"
	"github.com/Eyevinn/mp4ff/mp4"
)

// STPPInitSegment represents an initialization segment for STPP subtitle streams.
type STPPInitSegment struct {
	segment.BaseInitSegment
}

// Generate creates an initialization segment for STPP subtitle streams.
// It sets the language and time scale for the segment.
// It returns the generated initialization segment.
//
// If an error occurs during the generation process, it returns the error.
// The function also sets the language and time scale for the segment.
//
// Note: Subtitle encryption is not supported in this implementation.
func (s *STPPInitSegment) Generate() (*mp4.InitSegment, error) {
	init := segment.NewBaseInitSegment("audio", s.Lang, s.TimeScale, []string{"iso6", "piff"})
	init.AddEmptyTrack(s.TimeScale, "subtitle", s.Lang)

	trak := init.Moov.Trak
	err := trak.SetStppDescriptor("http://www.w3.org/ns/ttml http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt http://www.w3.org/ns/ttml#metadata  http://www.w3.org/ns/ttml#parameter http://www.w3.org/ns/ttml#styling http://www.w3.org/2001/XMLSchema-instance http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt.xsd", "", "")
	if err != nil {
		return nil, err
	}

	// we don't support encryption for stpp

	return init, nil
}
