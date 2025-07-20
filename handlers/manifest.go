package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/models"
	"github.com/Diniboy1123/manifesto/transformers"
)

// DashManifestHandler seamlessly transforms manifests to DASH format.
// It supports multiple source types (ISM, MPD) and transforms them to DASH manifests.
// It handles the request, fetches the source manifest, transforms it to DASH,
// and writes the DASH manifest to the response.
//
// The handler expects the channel information to be present in the request context.
// If the channel is not found in the context, it returns an error response.
//
// The source type is determined by the channel's SourceType field:
// - "ism": Microsoft Smooth Streaming manifest
// - "mpd": MPEG-DASH manifest (proxy mode)
//
// If any error occurs during the fetching or transformation process, it logs the error
// and returns an error response to the client.
//
// The handler also sets the Content-Type header to "application/dash+xml" and writes
// the transformed DASH manifest to the response body.
func DashManifestHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	manifestFetchStartTime := time.Now()
	
	var mpd *models.MPD
	var err error
	
	// Route based on source type
	switch channel.SourceType {
	case "ism", "": // Default to ISM for backward compatibility
		smoothStream, fetchErr := transformers.GetSmoothManifest(channel.Url)
		if fetchErr != nil {
			http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
			log.Printf("Error fetching ISM manifest: %v", fetchErr)
			return
		}
		manifestFetchTook := time.Since(manifestFetchStartTime)

		var hasKeys bool
		if channel.Keys != nil {
			hasKeys = true
		}

		manifestTransformStartTime := time.Now()
		mpd, err = transformers.SmoothToDashManifest(smoothStream, hasKeys, config.Get().AllowSubs, channel)
		if err != nil {
			http.Error(w, "Error transforming manifest", http.StatusInternalServerError)
			log.Printf("Error transforming ISM manifest: %v", err)
			return
		}
		logManifestTiming(w, r, manifestFetchTook, time.Since(manifestTransformStartTime))
		
	case "mpd":
		originalMPD, fetchErr := transformers.GetMPDManifest(channel.Url)
		if fetchErr != nil {
			http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
			log.Printf("Error fetching MPD manifest: %v", fetchErr)
			return
		}
		manifestFetchTook := time.Since(manifestFetchStartTime)

		manifestTransformStartTime := time.Now()
		mpd, err = transformers.MPDToMPDManifest(originalMPD, channel)
		if err != nil {
			http.Error(w, "Error transforming manifest", http.StatusInternalServerError)
			log.Printf("Error transforming MPD manifest: %v", err)
			return
		}
		logManifestTiming(w, r, manifestFetchTook, time.Since(manifestTransformStartTime))
		
	default:
		http.Error(w, fmt.Sprintf("Unsupported source type: %s", channel.SourceType), http.StatusBadRequest)
		log.Printf("Unsupported source type: %s for channel %s", channel.SourceType, channel.Id)
		return
	}

	mpdXML, err := mpd.Encode()
	if err != nil {
		http.Error(w, "Error encoding manifest", http.StatusInternalServerError)
		log.Printf("Error encoding manifest: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/dash+xml")
	w.Header().Set("Content-Length", strconv.Itoa(len(mpdXML)))
	w.WriteHeader(http.StatusOK)
	w.Write(mpdXML)
}

// logManifestTiming logs timing information for manifest processing
func logManifestTiming(w http.ResponseWriter, r *http.Request, fetchDuration, transformDuration time.Duration) {
	reqStartTime := r.Context().Value("reqStartTime").(time.Time)
	reqTook := time.Since(reqStartTime)

	w.Header().Set("Server-Timing", fmt.Sprintf(
		"manifest-fetch;dur=%.3f,manifest-transform;dur=%.3f,total;dur=%.3f",
		fetchDuration.Seconds()*1000,
		transformDuration.Seconds()*1000,
		reqTook.Seconds()*1000,
	))
}
