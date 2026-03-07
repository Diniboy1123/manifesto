package transformers

import (
	"strings"
	"testing"

	"github.com/Diniboy1123/manifesto/models"
)

func TestSmoothToHlsMasterPlaylist(t *testing.T) {
	ism := &models.SmoothStream{
		TimeScale: 10000000,
		IsLive:    false,
		Duration:  400000000,
		StreamIndexes: []models.StreamIndex{
			{
				Type:     "video",
				Name:     "video",
				Language: "und",
				Url:      "QualityLevels({bitrate})/Fragments(video={start time})",
				QualityLevels: []models.QualityLevel{
					{
						Index:            0,
						Bitrate:          5000000,
						MaxWidth:         1920,
						MaxHeight:        1080,
						FourCC:           "H264",
						CodecPrivateData: "00000001674d40209e5281806f60284040405000000300100000064e00000d1f400068fa3f13e0a00000000168ef7520",
					},
					{
						Index:            1,
						Bitrate:          3000000,
						MaxWidth:         1280,
						MaxHeight:        720,
						FourCC:           "H264",
						CodecPrivateData: "00000001674d40209e5281806f60284040405000000300100000064e00000d1f400068fa3f13e0a00000000168ef7520",
					},
				},
				ChunkInfos: []models.ChunkInfos{
					{Duration: 20000000, StartTime: 0},
					{Duration: 20000000},
				},
			},
			{
				Type:     "audio",
				Name:     "audio_eng",
				Language: "eng",
				Url:      "QualityLevels({bitrate})/Fragments(audio_eng={start time})",
				QualityLevels: []models.QualityLevel{
					{
						Index:        0,
						Bitrate:      128000,
						FourCC:       "AACL",
						SamplingRate: 48000,
						Channels:     2,
					},
				},
				ChunkInfos: []models.ChunkInfos{
					{Duration: 20000000, StartTime: 0},
					{Duration: 20000000},
				},
			},
			{
				Type:     "audio",
				Name:     "audio_deu",
				Language: "deu",
				Url:      "QualityLevels({bitrate})/Fragments(audio_deu={start time})",
				QualityLevels: []models.QualityLevel{
					{
						Index:        0,
						Bitrate:      128000,
						FourCC:       "AACL",
						SamplingRate: 48000,
						Channels:     2,
					},
				},
				ChunkInfos: []models.ChunkInfos{
					{Duration: 20000000, StartTime: 0},
					{Duration: 20000000},
				},
			},
		},
	}

	playlist, err := SmoothToHlsMasterPlaylist(ism, false)
	if err != nil {
		t.Fatalf("Failed to generate master playlist: %v", err)
	}

	content := string(playlist)

	// Check header tags
	if !strings.Contains(content, "#EXTM3U") {
		t.Error("Missing #EXTM3U header")
	}
	if !strings.Contains(content, "#EXT-X-VERSION:3") {
		t.Error("Missing #EXT-X-VERSION:3")
	}

	// Check video variants
	if !strings.Contains(content, "#EXT-X-STREAM-INF:BANDWIDTH=5000000") {
		t.Error("Missing first video stream info")
	}
	if !strings.Contains(content, "#EXT-X-STREAM-INF:BANDWIDTH=3000000") {
		t.Error("Missing second video stream info")
	}
	if !strings.Contains(content, "video_0/playlist.m3u8") {
		t.Error("Missing first video playlist URL")
	}
	if !strings.Contains(content, "video_1/playlist.m3u8") {
		t.Error("Missing second video playlist URL")
	}
	if !strings.Contains(content, "RESOLUTION=1920x1080") {
		t.Error("Missing resolution for first video")
	}
	if !strings.Contains(content, "RESOLUTION=1280x720") {
		t.Error("Missing resolution for second video")
	}

	// Check audio renditions
	if !strings.Contains(content, "#EXT-X-MEDIA:TYPE=AUDIO") {
		t.Error("Missing audio media tag")
	}
	if !strings.Contains(content, "audio_eng_0/playlist.m3u8") {
		t.Error("Missing English audio playlist URL")
	}
	if !strings.Contains(content, "audio_deu_0/playlist.m3u8") {
		t.Error("Missing German audio playlist URL")
	}
	if !strings.Contains(content, "LANGUAGE=\"eng\"") {
		t.Error("Missing English language tag")
	}
	if !strings.Contains(content, "LANGUAGE=\"deu\"") {
		t.Error("Missing German language tag")
	}
	if !strings.Contains(content, "AUDIO=\"audio\"") {
		t.Error("Missing AUDIO group reference in stream info")
	}

	// Check codecs
	if !strings.Contains(content, "CODECS=") {
		t.Error("Missing CODECS attribute")
	}
	if !strings.Contains(content, "mp4a.40.2") {
		t.Error("Missing AAC codec in CODECS")
	}

	// Only one audio should be DEFAULT=YES
	defaultCount := strings.Count(content, "DEFAULT=YES")
	if defaultCount != 1 {
		t.Errorf("Expected 1 default audio, got %d", defaultCount)
	}

	// Subtitles should not appear in HLS TS playlists
	if strings.Contains(content, "SUBTITLES") {
		t.Error("Subtitles should not be present in HLS TS playlists")
	}

	t.Logf("Generated master playlist:\n%s", content)
}

func TestSmoothToHlsMediaPlaylistVOD(t *testing.T) {
	ism := &models.SmoothStream{
		TimeScale: 10000000,
		IsLive:    false,
		Duration:  400000000,
		StreamIndexes: []models.StreamIndex{
			{
				Type:     "video",
				Name:     "video",
				Language: "und",
				Url:      "QualityLevels({bitrate})/Fragments(video={start time})",
				QualityLevels: []models.QualityLevel{
					{
						Index:   0,
						Bitrate: 5000000,
					},
				},
				ChunkInfos: []models.ChunkInfos{
					{Duration: 20000000, StartTime: 0},
					{Duration: 20000000},
				},
			},
		},
	}

	playlist, err := SmoothToHlsMediaPlaylist(ism, "video", 0)
	if err != nil {
		t.Fatalf("Failed to generate media playlist: %v", err)
	}

	content := string(playlist)

	if !strings.Contains(content, "#EXTM3U") {
		t.Error("Missing #EXTM3U header")
	}
	if !strings.Contains(content, "#EXT-X-VERSION:3") {
		t.Error("Missing version")
	}
	if !strings.Contains(content, "#EXT-X-TARGETDURATION:2") {
		t.Error("Missing or incorrect target duration")
	}
	if !strings.Contains(content, "#EXT-X-PLAYLIST-TYPE:VOD") {
		t.Error("Missing VOD playlist type")
	}
	if strings.Contains(content, "#EXT-X-MAP") {
		t.Error("TS playlists should not have EXT-X-MAP")
	}
	if !strings.Contains(content, "#EXTINF:2.000,") {
		t.Error("Missing EXTINF tag")
	}
	if !strings.Contains(content, "0/seg.ts") {
		t.Error("Missing first segment URL")
	}
	if !strings.Contains(content, "20000000/seg.ts") {
		t.Error("Missing second segment URL")
	}
	if !strings.Contains(content, "#EXT-X-ENDLIST") {
		t.Error("Missing ENDLIST for VOD")
	}

	t.Logf("Generated media playlist:\n%s", content)
}

func TestSmoothToHlsMediaPlaylistLive(t *testing.T) {
	ism := &models.SmoothStream{
		TimeScale: 10000000,
		IsLive:    true,
		StreamIndexes: []models.StreamIndex{
			{
				Type:     "audio",
				Name:     "audio",
				Language: "eng",
				Url:      "QualityLevels({bitrate})/Fragments(audio={start time})",
				QualityLevels: []models.QualityLevel{
					{
						Index:   0,
						Bitrate: 128000,
					},
				},
				ChunkInfos: []models.ChunkInfos{
					{Duration: 20000000, StartTime: 1000},
					{Duration: 20000000},
				},
			},
		},
	}

	playlist, err := SmoothToHlsMediaPlaylist(ism, "audio", 0)
	if err != nil {
		t.Fatalf("Failed to generate media playlist: %v", err)
	}

	content := string(playlist)

	if strings.Contains(content, "#EXT-X-PLAYLIST-TYPE:VOD") {
		t.Error("Live stream should not have VOD playlist type")
	}
	if strings.Contains(content, "#EXT-X-ENDLIST") {
		t.Error("Live stream should not have ENDLIST tag")
	}
	if !strings.Contains(content, "1000/seg.ts") {
		t.Error("Missing first segment with correct start time")
	}

	t.Logf("Generated live media playlist:\n%s", content)
}

func TestSmoothToHlsMediaPlaylistInvalidStream(t *testing.T) {
	ism := &models.SmoothStream{
		TimeScale: 10000000,
		StreamIndexes: []models.StreamIndex{
			{
				Type: "video",
				Name: "video",
				QualityLevels: []models.QualityLevel{
					{Index: 0, Bitrate: 5000000},
				},
			},
		},
	}

	_, err := SmoothToHlsMediaPlaylist(ism, "nonexistent", 0)
	if err == nil {
		t.Error("Expected error for non-existent stream index")
	}

	_, err = SmoothToHlsMediaPlaylist(ism, "video", 99)
	if err == nil {
		t.Error("Expected error for non-existent quality level")
	}
}

func TestBoolToYesNo(t *testing.T) {
	if boolToYesNo(true) != "YES" {
		t.Error("Expected YES for true")
	}
	if boolToYesNo(false) != "NO" {
		t.Error("Expected NO for false")
	}
}
