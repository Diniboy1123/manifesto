package transformers

import (
	"fmt"
	"strings"
	"time"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
	"github.com/Diniboy1123/manifesto/models"
)

// GetMPDManifest requests the MPD manifest from the given URL and parses it into an MPD object
//
// If the request fails, it returns an error.
func GetMPDManifest(url string) (*models.MPD, error) {
	content, err := utils.DoRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer content.Body.Close()

	mpdManifest, err := models.NewMPDFromReader(content.Body)
	if err != nil {
		return nil, err
	}

	return mpdManifest, nil
}

// MPDToMPDManifest processes an MPD manifest to rewrite URLs for proxying through manifesto.
// It takes the original MPD manifest and a channel configuration.
// It returns the modified DASH manifest with URLs rewritten to point through our proxy.
//
// The function processes the MPD manifest, rewriting segment and initialization URLs
// to route through our proxy server. This allows manifesto to act as a proxy for
// MPD sources, enabling features like caching, authentication, and URL transformation.
func MPDToMPDManifest(originalMPD *models.MPD, channel config.Channel) (*models.MPD, error) {
	// Create a copy of the MPD to avoid modifying the original
	modifiedMPD := *originalMPD
	
	// Update timestamps for live streams
	if modifiedMPD.Type == "dynamic" {
		now := time.Now().UTC()
		modifiedMPD.PublishTime = now.Format("2006-01-02T15:04:05Z")
		
		if modifiedMPD.UTCTiming != nil {
			// Create a copy of UTCTiming to avoid modifying the original
			utcTiming := *modifiedMPD.UTCTiming
			utcTiming.Value = now.Format("2006-01-02T15:04:05Z")
			modifiedMPD.UTCTiming = &utcTiming
		}
	}

	// Update program information if configured
	if channel.Name != "" && modifiedMPD.ProgramInformation != nil {
		// Create a copy of ProgramInformation to avoid modifying the original
		progInfo := *modifiedMPD.ProgramInformation
		progInfo.Title = channel.Name
		modifiedMPD.ProgramInformation = &progInfo
	}

	// Process periods and adaptation sets to rewrite URLs
	for _, period := range modifiedMPD.Period {
		for _, adaptationSet := range period.AdaptationSets {
			// Rewrite SegmentTemplate URLs if present
			if adaptationSet.SegmentTemplate != nil {
				adaptationSet.SegmentTemplate = rewriteSegmentTemplate(adaptationSet.SegmentTemplate, adaptationSet.ID)
			}
			
			// Process representations
			for _, representation := range adaptationSet.Representations {
				// Rewrite SegmentTemplate URLs at representation level
				if representation.SegmentTemplate != nil {
					representation.SegmentTemplate = rewriteSegmentTemplate(representation.SegmentTemplate, representation.ID)
				}
				
				// Rewrite BaseURL if present
				for _, baseURL := range representation.BaseURL {
					baseURL.Value = rewriteBaseURL(baseURL.Value, representation.ID)
				}
			}
		}
	}

	return &modifiedMPD, nil
}

// rewriteSegmentTemplate modifies SegmentTemplate URLs to route through our proxy
func rewriteSegmentTemplate(segmentTemplate *models.SegmentTemplate, representationID string) *models.SegmentTemplate {
	newTemplate := *segmentTemplate
	
	if newTemplate.Media != "" {
		// Rewrite media template to route through our proxy
		newTemplate.Media = fmt.Sprintf("%s/$Time$/%s", representationID, convertMPDToProxyURL(newTemplate.Media))
	}
	
	if newTemplate.Initialization != "" {
		// Rewrite initialization template to route through our proxy  
		newTemplate.Initialization = fmt.Sprintf("%s/init.mp4", representationID)
	}
	
	return &newTemplate
}

// rewriteBaseURL modifies BaseURL to route through our proxy
func rewriteBaseURL(baseURL, representationID string) string {
	if baseURL == "" {
		return baseURL
	}
	
	// For BaseURL, we need to create a proxy path
	return fmt.Sprintf("%s/base/%s", representationID, convertMPDToProxyURL(baseURL))
}

// convertMPDToProxyURL converts MPD template variables to our proxy URL format
func convertMPDToProxyURL(url string) string {
	// Convert MPD template variables to a format we can handle
	// This is a basic conversion - more sophisticated logic may be needed
	// depending on the specific MPD templates encountered
	replacer := strings.NewReplacer(
		"$RepresentationID$", "{representationId}",
		"$Time$", "{time}",
		"$Number$", "{number}",
		"$Bandwidth$", "{bandwidth}",
	)
	return replacer.Replace(url)
}