package models

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"

	"github.com/unki2aut/go-xsd-types"
)

type UTCTiming struct {
	SchemeIdUri string `xml:"schemeIdUri,attr"`
	Value       string `xml:"value,attr"`
}

type MPD struct {
	XMLNS                      string        `xml:"xmlns,attr"`
	Profiles                   string        `xml:"profiles,attr"`
	XMLNSCommonEncryption      string        `xml:"xmlns:cenc,attr,omitempty"`
	XMLNSPlayReady             string        `xml:"xmlns:mspr,attr,omitempty"`
	Type                       string        `xml:"type,attr"`
	MinBufferTime              *xsd.Duration `xml:"minBufferTime,attr"`
	AvailabilityStartTime      string        `xml:"availabilityStartTime,attr"`
	MinimumUpdatePeriod        *xsd.Duration `xml:"minimumUpdatePeriod,attr"`
	PublishTime                string        `xml:"publishTime,attr"`
	TimeShiftBufferDepth       *xsd.Duration `xml:"timeShiftBufferDepth,attr"`
	AvailabilityEndTime        string        `xml:"availabilityEndTime,attr,omitempty"`
	MediaPresentationDuration  *xsd.Duration `xml:"mediaPresentationDuration,attr"`
	SuggestedPresentationDelay *xsd.Duration `xml:"suggestedPresentationDelay,attr"`
	BaseURL                    []*BaseURL    `xml:"BaseURL,omitempty"`
	Period                     []*Period     `xml:"Period,omitempty"`
	UTCTiming                  *UTCTiming    `xml:"UTCTiming,omitempty"`
}

type BaseURL struct {
	Value                    string  `xml:",chardata"`
	ServiceLocation          *string `xml:"serviceLocation,attr"`
	ByteRange                *string `xml:"byteRange,attr"`
	AvailabilityTimeOffset   *uint64 `xml:"availabilityTimeOffset,attr"`
	AvailabilityTimeComplete *bool   `xml:"availabilityTimeComplete,attr"`
}

type Period struct {
	Start          *xsd.Duration    `xml:"start,attr"`
	ID             string           `xml:"id,attr"`
	Duration       *xsd.Duration    `xml:"duration,attr"`
	AdaptationSets []*AdaptationSet `xml:"AdaptationSet,omitempty"`
	BaseURL        []*BaseURL       `xml:"BaseURL,omitempty"`
}

type AdaptationSet struct {
	MimeType                  string                     `xml:"mimeType,attr"`
	StartWithSAP              ConditionalUint            `xml:"startWithSAP,attr"`
	ID                        string                     `xml:"id,attr"`
	SegmentAlignment          ConditionalUint            `xml:"segmentAlignment,attr"`
	Lang                      string                     `xml:"lang,attr"`
	ContentType               string                     `xml:"contentType,attr"`
	SubsegmentAlignment       ConditionalUint            `xml:"subsegmentAlignment,attr"`
	SubsegmentStartsWithSAP   ConditionalUint            `xml:"subsegmentStartsWithSAP,attr"`
	BitstreamSwitching        *bool                      `xml:"bitstreamSwitching,attr"`
	Par                       string                     `xml:"par,attr,omitempty"`
	Codecs                    string                     `xml:"codecs,attr,omitempty"`
	Role                      []*Descriptor              `xml:"Role,omitempty"`
	BaseURL                   []*BaseURL                 `xml:"BaseURL,omitempty"`
	AudioChannelConfiguration *AudioChannelConfiguration `xml:"AudioChannelConfiguration,omitempty"`
	ContentProtections        []Descriptor               `xml:"ContentProtection,omitempty"`
	SegmentTemplate           *SegmentTemplate           `xml:"SegmentTemplate,omitempty"`
	Representations           []*Representation          `xml:"Representation,omitempty"`
}

type AudioChannelConfiguration struct {
	SchemeIdUri string `xml:"schemeIdUri,attr"`
	Value       string `xml:"value,attr"`
}

type Representation struct {
	ID                 string           `xml:"id,attr"`
	Width              uint64           `xml:"width,attr,omitempty"`
	Height             uint64           `xml:"height,attr,omitempty"`
	FrameRate          string           `xml:"frameRate,attr,omitempty"`
	Bandwidth          uint64           `xml:"bandwidth,attr"`
	AudioSamplingRate  string           `xml:"audioSamplingRate,attr,omitempty"`
	Codecs             string           `xml:"codecs,attr"`
	SAR                string           `xml:"sar,attr,omitempty"`
	ScanType           string           `xml:"scanType,attr,omitempty"`
	ContentProtections []Descriptor     `xml:"ContentProtection,omitempty"`
	SegmentTemplate    *SegmentTemplate `xml:"SegmentTemplate,omitempty"`
	BaseURL            []*BaseURL       `xml:"BaseURL,omitempty"`
}

type Descriptor struct {
	Value       string `xml:"value,attr,omitempty"`
	SchemeIDURI string `xml:"schemeIdUri,attr"`
	Pro         *Pro   `xml:"mspr:pro,omitempty"`
	Pssh        *Pssh  `xml:"cenc:pssh,omitempty"`
}

type SegmentTemplate struct {
	Duration               uint64           `xml:"duration,attr,omitempty"`
	Initialization         string           `xml:"initialization,attr"`
	Media                  string           `xml:"media,attr"`
	Timescale              uint64           `xml:"timescale,attr"`
	StartNumber            uint64           `xml:"startNumber,attr,omitempty"`
	PresentationTimeOffset uint64           `xml:"presentationTimeOffset,attr,omitempty"`
	SegmentTimeline        *SegmentTimeline `xml:"SegmentTimeline,omitempty"`
}

type SegmentTimeline struct {
	S []SegmentTimelineS `xml:"S"`
}

type SegmentTimelineS struct {
	T uint64 `xml:"t,attr,omitempty"`
	D uint64 `xml:"d,attr"`
	R int64  `xml:"r,attr,omitempty"`
}

type Pro struct {
	XMLNS string `xml:"xmlns:mspr,attr"`
	Data  string `xml:",chardata"`
}

type Pssh struct {
	XMLNS string `xml:"xmlns:cenc,attr"`
	Data  string `xml:",chardata"`
}

var emptyElementRE = regexp.MustCompile(`></[A-Za-z]+>`)

// Encode encodes the MPD object to XML format
// and returns the byte representation of the XML
// with the XML declaration prepended.
// It also replaces empty elements with self-closing tags.
func (m *MPD) Encode() ([]byte, error) {
	x := new(bytes.Buffer)
	e := xml.NewEncoder(x)
	e.Indent("", "  ")
	err := e.Encode(m)
	if err != nil {
		return nil, err
	}

	res := new(bytes.Buffer)
	res.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	res.WriteByte('\n')
	for {
		s, err := x.ReadString('\n')
		if s != "" {
			s = emptyElementRE.ReplaceAllString(s, `/>`)
			res.WriteString(s)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	res.WriteByte('\n')
	return res.Bytes(), err
}
