package models

import (
	"encoding/xml"
	"io"
	"strings"
)

type SmoothStream struct {
	XMLName                xml.Name                 `xml:"SmoothStreamingMedia"`
	MajorVersion           int                      `xml:"MajorVersion,attr"`
	MinorVersion           int                      `xml:"MinorVersion,attr"`
	Duration               uint64                   `xml:"Duration,attr"`
	TimeScale              uint64                   `xml:"TimeScale,attr"`
	IsLive                 bool                     `xml:"IsLive,attr"`
	LookAheadFragmentCount int                      `xml:"LookAheadFragmentCount,attr"`
	DVRWindowLength        int64                    `xml:"DVRWindowLength,attr"`
	CanSeek                bool                     `xml:"CanSeek,attr"`
	CanPause               bool                     `xml:"CanPause,attr"`
	Protection             []SmoothProtectionHeader `xml:"Protection>ProtectionHeader"`
	StreamIndexes          []StreamIndex            `xml:"StreamIndex"`
}

type SmoothProtectionHeader struct {
	XMLName    xml.Name `xml:"ProtectionHeader"`
	SystemID   string   `xml:"SystemID,attr"`
	CustomData string   `xml:",chardata"`
}

type StreamIndex struct {
	XMLName       xml.Name       `xml:"StreamIndex"`
	Type          string         `xml:"Type,attr"`
	Name          string         `xml:"Name,attr"`
	Language      string         `xml:"Language,attr"`
	Subtype       string         `xml:"Subtype,attr"`
	Chunks        int            `xml:"Chunks,attr"`
	TimeScale     int            `xml:"TimeScale,attr"`
	Url           string         `xml:"Url,attr"`
	QualityLevels []QualityLevel `xml:"QualityLevel"`
	ChunkInfos    []ChunkInfos   `xml:"c"`
}

type QualityLevel struct {
	XMLName          xml.Name `xml:"QualityLevel"`
	Index            int      `xml:"Index,attr"`
	Bitrate          uint64   `xml:"Bitrate,attr"`
	CodecPrivateData string   `xml:"CodecPrivateData,attr"`
	FourCC           string   `xml:"FourCC,attr"`
	MaxWidth         uint64   `xml:"MaxWidth,attr"`
	MaxHeight        uint64   `xml:"MaxHeight,attr"`
	AudioTag         int      `xml:"AudioTag,attr"`
	Channels         int      `xml:"Channels,attr"`
	SamplingRate     int64    `xml:"SamplingRate,attr"`
	BitsPerSample    int      `xml:"BitsPerSample,attr"`
	PacketSize       int      `xml:"PacketSize,attr"`
}

type ChunkInfos struct {
	XMLName   xml.Name `xml:"c"`
	Duration  uint64   `xml:"d,attr"`
	StartTime uint64   `xml:"t,attr"`
}

// SmoothStreamError represents an error in the smoothstreaming manifest parsing process.
type SmoothStreamError struct {
	Err string
}

func (e *SmoothStreamError) Error() string {
	return e.Err
}

// NewSmoothStreamError creates a new SmoothStreamError with the given error message.
func NewSmoothStreamError(err string) *SmoothStreamError {
	return &SmoothStreamError{err}
}

// NewSmoothStream creates a new SmoothStream instance by decoding the XML data from the provided io.Reader.
// It returns a pointer to the SmoothStream instance and an error if any occurred during decoding.
func NewSmoothStream(r io.Reader) (*SmoothStream, error) {
	var ss SmoothStream
	decoder := xml.NewDecoder(r)
	err := decoder.Decode(&ss)
	if err != nil {
		return nil, err
	}
	return &ss, nil
}

// GetStreamIndexByNameOrType retrieves a stream index by its name or type.
// It returns a pointer to the StreamIndex.
// The name parameter specifies the name of the stream index to retrieve.
//
// If no stream index is found with the specified name or type, it returns an error.
func (ss *SmoothStream) GetStreamIndexByNameOrType(name string) (*StreamIndex, error) {
	for _, si := range ss.StreamIndexes {
		if si.Name == name {
			return &si, nil
		}
	}
	for _, si := range ss.StreamIndexes {
		if si.Type == name {
			return &si, nil
		}
	}
	return nil, NewSmoothStreamError("no stream index found with the specified name")
}

// GetMimeType retrieves the MIME type for a given stream index.
//
// If not found, it returns "application/octet-stream" as the default MIME type.
func (si *StreamIndex) GetMimeType() string {
	switch si.Type {
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mp4"
	case "text":
		return "application/mp4"
	default:
		return "application/octet-stream"
	}
}

// GetQualityLevelByIndex retrieves a quality level by its index.
// It returns a pointer to the QualityLevel.
// The index parameter specifies the index of the quality level to retrieve.
//
// If no quality level is found with the specified index, it returns an error.
func (si *StreamIndex) GetQualityLevelByIndex(index int) (*QualityLevel, error) {
	for _, ql := range si.QualityLevels {
		if ql.Index == index {
			return &ql, nil
		}
	}
	return nil, NewSmoothStreamError("no quality level found with the specified index")
}

// GetProtectionHeaderForSystemId retrieves the protection header for a given system ID.
// It returns a pointer to the SmoothProtectionHeader.
// The systemId parameter specifies the system ID of the protection header to retrieve.
//
// If no protection header is found with the specified system ID, it returns nil.
func (ss *SmoothStream) GetProtectionHeaderForSystemId(systemId string) *SmoothProtectionHeader {
	for _, ph := range ss.Protection {
		if strings.EqualFold(ph.SystemID, systemId) {
			return &ph
		}
	}
	return nil
}
