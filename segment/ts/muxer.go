package ts

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	astits "github.com/asticode/go-astits"
	"github.com/Eyevinn/mp4ff/mp4"
)

const (
	videoPID uint16 = 256
	audioPID uint16 = 257

	// 90kHz is the standard MPEG-TS clock rate
	tsClockRate int64 = 90000

	videoStreamID uint8 = 0xe0
	audioStreamID uint8 = 0xc0
)

// sampleInfo holds extracted sample data and timing from an fMP4 fragment.
type sampleInfo struct {
	data     []byte
	dts      uint64 // decode time stamp in original timescale
	pts      int64  // presentation time stamp in original timescale
	duration uint32
	isSync   bool
}

// extractSamplesFromFMP4 decodes an fMP4 segment, optionally decrypts it,
// and extracts individual media samples with timing information.
func extractSamplesFromFMP4(mp4Data []byte, decryptInfo mp4.DecryptInfo, key []byte, chunkTime uint64) ([]sampleInfo, error) {
	inMp4, err := mp4.DecodeFile(bytes.NewReader(mp4Data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode fMP4: %w", err)
	}

	if !inMp4.IsFragmented() {
		return nil, fmt.Errorf("input is not fragmented MP4")
	}

	for _, seg := range inMp4.Segments {
		for _, frag := range seg.Fragments {
			// Required for proper decryption
			frag.Moof.Traf.Tfhd.TrackID = 1
			frag.Moof.Traf.Trun.DataOffset = 0
		}

		if key != nil {
			if err := mp4.DecryptSegment(seg, decryptInfo, key); err != nil {
				return nil, fmt.Errorf("failed to decrypt segment: %w", err)
			}
		}
	}

	var samples []sampleInfo

	for _, seg := range inMp4.Segments {
		for _, frag := range seg.Fragments {
			var baseDecodeTime uint64
			if frag.Moof.Traf.Tfdt != nil {
				baseDecodeTime = frag.Moof.Traf.Tfdt.BaseMediaDecodeTime()
			} else {
				baseDecodeTime = chunkTime
			}

			for _, trun := range frag.Moof.Traf.Truns {
				fullSamples := trun.GetFullSamples(0, baseDecodeTime, frag.Mdat)
				for _, fs := range fullSamples {
					samples = append(samples, sampleInfo{
						data:     fs.Data,
						dts:      fs.DecodeTime,
						pts:      int64(fs.PresentationTime()),
						duration: fs.Dur,
						isSync:   fs.IsSync(),
					})
				}
			}
		}
	}

	return samples, nil
}

// MuxVideoSegmentToTS converts an fMP4 video segment to MPEG-TS format.
// It extracts H.264 samples, converts them from AVCC to Annex B format,
// prepends SPS/PPS NALUs at keyframes, and writes TS packets using go-astits.
func MuxVideoSegmentToTS(ctx context.Context, w io.Writer, mp4Data []byte,
	decryptInfo mp4.DecryptInfo, key []byte,
	spsNALUs, ppsNALUs [][]byte, timeScale uint32, chunkTime uint64) error {

	samples, err := extractSamplesFromFMP4(mp4Data, decryptInfo, key, chunkTime)
	if err != nil {
		return err
	}

	muxer := astits.NewMuxer(ctx, w)
	if err := muxer.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: videoPID,
		StreamType:    astits.StreamTypeH264Video,
	}); err != nil {
		return fmt.Errorf("failed to add video elementary stream: %w", err)
	}
	muxer.SetPCRPID(videoPID)

	for _, sample := range samples {
		// Convert AVCC format to Annex B (start code delimited)
		annexB := avccToAnnexB(sample.data)

		// Prepend SPS/PPS at sync (key) samples for decoder initialization
		if sample.isSync && len(spsNALUs) > 0 {
			annexB = prependParameterSets(spsNALUs, ppsNALUs, annexB)
		}

		// Convert timing from source timescale to 90kHz TS clock
		pts90 := sample.pts * tsClockRate / int64(timeScale)
		dts90 := int64(sample.dts) * tsClockRate / int64(timeScale)

		if _, err := muxer.WriteData(&astits.MuxerData{
			PID: videoPID,
			AdaptationField: &astits.PacketAdaptationField{
				RandomAccessIndicator: sample.isSync,
			},
			PES: &astits.PESData{
				Header: &astits.PESHeader{
					OptionalHeader: &astits.PESOptionalHeader{
						PTS:                    &astits.ClockReference{Base: pts90},
						DTS:                    &astits.ClockReference{Base: dts90},
						DataAlignmentIndicator: true,
						MarkerBits:             2,
					},
					StreamID: videoStreamID,
				},
				Data: annexB,
			},
		}); err != nil {
			return fmt.Errorf("failed to write video TS data: %w", err)
		}
	}

	return nil
}

// MuxAudioAACSegmentToTS converts an fMP4 AAC audio segment to MPEG-TS format.
// It extracts raw AAC frames, prepends ADTS headers, and writes TS packets.
func MuxAudioAACSegmentToTS(ctx context.Context, w io.Writer, mp4Data []byte,
	decryptInfo mp4.DecryptInfo, key []byte,
	objectType byte, sampleRate int, channelConfig byte,
	timeScale uint32, chunkTime uint64) error {

	samples, err := extractSamplesFromFMP4(mp4Data, decryptInfo, key, chunkTime)
	if err != nil {
		return err
	}

	sampleRateIdx, ok := sampleRateToIndex(sampleRate)
	if !ok {
		return fmt.Errorf("unsupported AAC sample rate: %d", sampleRate)
	}

	muxer := astits.NewMuxer(ctx, w)
	if err := muxer.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: audioPID,
		StreamType:    astits.StreamTypeAACAudio,
	}); err != nil {
		return fmt.Errorf("failed to add audio elementary stream: %w", err)
	}
	muxer.SetPCRPID(audioPID)

	for _, sample := range samples {
		// Create ADTS frame (header + raw AAC data)
		adtsFrame := createADTSFrame(objectType, sampleRateIdx, channelConfig, sample.data)

		pts90 := sample.pts * tsClockRate / int64(timeScale)

		if _, err := muxer.WriteData(&astits.MuxerData{
			PID: audioPID,
			PES: &astits.PESData{
				Header: &astits.PESHeader{
					OptionalHeader: &astits.PESOptionalHeader{
						PTS:                    &astits.ClockReference{Base: pts90},
						DataAlignmentIndicator: true,
						MarkerBits:             2,
					},
					StreamID: audioStreamID,
				},
				Data: adtsFrame,
			},
		}); err != nil {
			return fmt.Errorf("failed to write AAC TS data: %w", err)
		}
	}

	return nil
}

// MuxAudioEAC3SegmentToTS converts an fMP4 EAC-3 (Dolby Digital Plus) audio segment to MPEG-TS format.
// EAC-3 frames are self-describing, so no additional headers are needed.
func MuxAudioEAC3SegmentToTS(ctx context.Context, w io.Writer, mp4Data []byte,
	decryptInfo mp4.DecryptInfo, key []byte,
	timeScale uint32, chunkTime uint64) error {

	samples, err := extractSamplesFromFMP4(mp4Data, decryptInfo, key, chunkTime)
	if err != nil {
		return err
	}

	muxer := astits.NewMuxer(ctx, w)
	if err := muxer.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: audioPID,
		StreamType:    astits.StreamTypeEAC3Audio,
	}); err != nil {
		return fmt.Errorf("failed to add EAC-3 elementary stream: %w", err)
	}
	muxer.SetPCRPID(audioPID)

	for _, sample := range samples {
		pts90 := sample.pts * tsClockRate / int64(timeScale)

		if _, err := muxer.WriteData(&astits.MuxerData{
			PID: audioPID,
			PES: &astits.PESData{
				Header: &astits.PESHeader{
					OptionalHeader: &astits.PESOptionalHeader{
						PTS:                    &astits.ClockReference{Base: pts90},
						DataAlignmentIndicator: true,
						MarkerBits:             2,
					},
					StreamID: audioStreamID,
				},
				Data: sample.data,
			},
		}); err != nil {
			return fmt.Errorf("failed to write EAC-3 TS data: %w", err)
		}
	}

	return nil
}

// avccToAnnexB converts H.264 sample data from AVCC format (4-byte length-prefixed NALUs)
// to Annex B format (start code-separated NALUs) as required by MPEG-TS.
func avccToAnnexB(avcc []byte) []byte {
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	var result bytes.Buffer

	offset := 0
	for offset < len(avcc) {
		if offset+4 > len(avcc) {
			break
		}
		naluLen := int(binary.BigEndian.Uint32(avcc[offset:]))
		offset += 4

		if naluLen <= 0 || offset+naluLen > len(avcc) {
			break
		}

		result.Write(startCode)
		result.Write(avcc[offset : offset+naluLen])
		offset += naluLen
	}

	return result.Bytes()
}

// prependParameterSets prepends SPS and PPS NALUs (in Annex B format) before the sample data.
// This is needed at keyframes so the decoder can initialize properly.
func prependParameterSets(spsNALUs, ppsNALUs [][]byte, annexBData []byte) []byte {
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	var result bytes.Buffer

	for _, sps := range spsNALUs {
		result.Write(startCode)
		result.Write(sps)
	}
	for _, pps := range ppsNALUs {
		result.Write(startCode)
		result.Write(pps)
	}
	result.Write(annexBData)

	return result.Bytes()
}

// createADTSFrame creates an ADTS frame by prepending a 7-byte ADTS header to raw AAC data.
// ADTS headers are required for AAC audio in MPEG-TS as the transport stream expects
// self-describing audio frames.
func createADTSFrame(objectType byte, sampleRateIndex byte, channelConfig byte, aacData []byte) []byte {
	frameLength := len(aacData) + 7 // 7-byte ADTS header (no CRC)

	header := make([]byte, 7)
	header[0] = 0xFF                                                                // Sync word (8 bits)
	header[1] = 0xF1                                                                // Sync word cont + MPEG-4 + Layer 0 + no CRC
	header[2] = ((objectType - 1) << 6) | (sampleRateIndex << 2) | ((channelConfig >> 2) & 0x01)
	header[3] = ((channelConfig & 0x03) << 6) | byte((frameLength>>11)&0x03)
	header[4] = byte((frameLength >> 3) & 0xFF)
	header[5] = byte(((frameLength & 0x07) << 5) | 0x1F)
	header[6] = 0xFC

	result := make([]byte, frameLength)
	copy(result, header)
	copy(result[7:], aacData)

	return result
}

// sampleRateToIndex maps AAC sample rates to their ADTS frequency index values.
func sampleRateToIndex(sampleRate int) (byte, bool) {
	rates := map[int]byte{
		96000: 0, 88200: 1, 64000: 2, 48000: 3,
		44100: 4, 32000: 5, 24000: 6, 22050: 7,
		16000: 8, 12000: 9, 11025: 10, 8000: 11,
		7350: 12,
	}
	idx, ok := rates[sampleRate]
	return idx, ok
}
