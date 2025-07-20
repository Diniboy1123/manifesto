package transformers

import (
	"strings"
	"testing"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/models"
)

func TestMPDToMPDManifest(t *testing.T) {
	// Create a sample MPD
	originalMPD := &models.MPD{
		XMLNS:    "urn:mpeg:dash:schema:mpd:2011",
		Type:     "static",
		Profiles: "urn:mpeg:dash:profile:isoff-main:2011",
		Period: []*models.Period{
			{
				ID: "0",
				AdaptationSets: []*models.AdaptationSet{
					{
						ID:       "0",
						MimeType: "video/mp4",
						SegmentTemplate: &models.SegmentTemplate{
							Media:          "video_$RepresentationID$_$Time$.m4s",
							Initialization: "video_$RepresentationID$_init.mp4",
						},
						Representations: []*models.Representation{
							{
								ID:        "video_1",
								Bandwidth: 1000000,
								SegmentTemplate: &models.SegmentTemplate{
									Media:          "video_1_$Time$.m4s",
									Initialization: "video_1_init.mp4",
								},
							},
						},
					},
				},
			},
		},
	}

	channel := config.Channel{
		Id:              "test-channel",
		Name:            "Test Channel",
		SourceType:      "mpd",
		DestinationType: "mpd",
		Url:             "https://example.com/manifest.mpd",
	}

	// Transform the MPD
	result, err := MPDToMPDManifest(originalMPD, channel)
	if err != nil {
		t.Fatalf("MPDToMPDManifest failed: %v", err)
	}

	// Verify the result
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// Check that basic properties are preserved
	if result.XMLNS != originalMPD.XMLNS {
		t.Errorf("Expected XMLNS %s, got %s", originalMPD.XMLNS, result.XMLNS)
	}

	if result.Type != originalMPD.Type {
		t.Errorf("Expected Type %s, got %s", originalMPD.Type, result.Type)
	}

	if result.Profiles != originalMPD.Profiles {
		t.Errorf("Expected Profiles %s, got %s", originalMPD.Profiles, result.Profiles)
	}

	// Check that periods are present
	if len(result.Period) != 1 {
		t.Fatalf("Expected 1 period, got %d", len(result.Period))
	}

	period := result.Period[0]
	if len(period.AdaptationSets) != 1 {
		t.Fatalf("Expected 1 adaptation set, got %d", len(period.AdaptationSets))
	}

	adaptationSet := period.AdaptationSets[0]
	if adaptationSet.SegmentTemplate == nil {
		t.Fatal("Expected SegmentTemplate to be present")
	}

	// Check that URLs have been rewritten
	expectedMedia := "0/$Time$/video_{representationId}_{time}.m4s"
	if adaptationSet.SegmentTemplate.Media != expectedMedia {
		t.Errorf("Expected Media URL %s, got %s", expectedMedia, adaptationSet.SegmentTemplate.Media)
	}

	expectedInit := "0/init.mp4"
	if adaptationSet.SegmentTemplate.Initialization != expectedInit {
		t.Errorf("Expected Initialization URL %s, got %s", expectedInit, adaptationSet.SegmentTemplate.Initialization)
	}

	// Check representation-level SegmentTemplate
	if len(adaptationSet.Representations) != 1 {
		t.Fatalf("Expected 1 representation, got %d", len(adaptationSet.Representations))
	}

	representation := adaptationSet.Representations[0]
	if representation.SegmentTemplate == nil {
		t.Fatal("Expected representation SegmentTemplate to be present")
	}

	expectedRepMedia := "video_1/$Time$/video_1_{time}.m4s"
	if representation.SegmentTemplate.Media != expectedRepMedia {
		t.Errorf("Expected representation Media URL %s, got %s", expectedRepMedia, representation.SegmentTemplate.Media)
	}

	expectedRepInit := "video_1/init.mp4"
	if representation.SegmentTemplate.Initialization != expectedRepInit {
		t.Errorf("Expected representation Initialization URL %s, got %s", expectedRepInit, representation.SegmentTemplate.Initialization)
	}
}

func TestMPDToMPDManifest_LiveStream(t *testing.T) {
	// Create a sample live MPD
	originalMPD := &models.MPD{
		XMLNS:       "urn:mpeg:dash:schema:mpd:2011",
		Type:        "dynamic",
		Profiles:    "urn:mpeg:dash:profile:isoff-live:2011",
		PublishTime: "2023-01-01T00:00:00Z",
		UTCTiming: &models.UTCTiming{
			SchemeIdUri: "urn:mpeg:dash:utc:direct:2014",
			Value:       "2023-01-01T00:00:00Z",
		},
	}

	channel := config.Channel{
		Id:              "test-live",
		Name:            "Test Live Channel",
		SourceType:      "mpd",
		DestinationType: "mpd",
		Url:             "https://example.com/live.mpd",
	}

	// Transform the MPD
	result, err := MPDToMPDManifest(originalMPD, channel)
	if err != nil {
		t.Fatalf("MPDToMPDManifest failed: %v", err)
	}

	// Verify that timestamps have been updated for live streams
	if result.PublishTime == originalMPD.PublishTime {
		t.Error("PublishTime should have been updated for live streams")
	}

	if result.UTCTiming != nil && result.UTCTiming.Value == originalMPD.UTCTiming.Value {
		t.Error("UTCTiming value should have been updated for live streams")
	}
}

func TestConvertMPDToProxyURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "video_$RepresentationID$_$Time$.m4s",
			expected: "video_{representationId}_{time}.m4s",
		},
		{
			input:    "chunk_$Number$_$Bandwidth$.mp4",
			expected: "chunk_{number}_{bandwidth}.mp4",
		},
		{
			input:    "simple_file.mp4",
			expected: "simple_file.mp4",
		},
	}

	for _, test := range tests {
		result := convertMPDToProxyURL(test.input)
		if result != test.expected {
			t.Errorf("convertMPDToProxyURL(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestNewMPDFromReader(t *testing.T) {
	mpdXML := `<?xml version="1.0" encoding="utf-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" profiles="urn:mpeg:dash:profile:isoff-main:2011">
  <Period id="0">
    <AdaptationSet id="0" mimeType="video/mp4">
      <Representation id="video_1" bandwidth="1000000"/>
    </AdaptationSet>
  </Period>
</MPD>`

	reader := strings.NewReader(mpdXML)
	mpd, err := models.NewMPDFromReader(reader)
	if err != nil {
		t.Fatalf("NewMPDFromReader failed: %v", err)
	}

	if mpd.Type != "static" {
		t.Errorf("Expected Type static, got %s", mpd.Type)
	}

	if len(mpd.Period) != 1 {
		t.Fatalf("Expected 1 period, got %d", len(mpd.Period))
	}

	if mpd.Period[0].ID != "0" {
		t.Errorf("Expected Period ID 0, got %s", mpd.Period[0].ID)
	}
}