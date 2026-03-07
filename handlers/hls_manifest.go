package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/transformers"
)

// HlsMasterHandler handles requests for the HLS master playlist (manifest.m3u8).
// It retrieves the SmoothStream manifest, transforms it into an HLS master playlist
// listing all available video variants and audio renditions.
//
// The handler generates playlists that reference MPEG-TS segments served by HlsSegmentHandler.
//
// The handler expects the channel information to be present in the request context.
func HlsMasterHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	manifestFetchStartTime := time.Now()
	smoothStream, err := transformers.GetSmoothManifest(channel.Url)
	if err != nil {
		http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
		log.Printf("Error fetching manifest: %v", err)
		return
	}
	manifestFetchTook := time.Since(manifestFetchStartTime)

	var hasKeys bool
	if channel.Keys != nil {
		hasKeys = true
	}

	manifestTransformStartTime := time.Now()
	playlist, err := transformers.SmoothToHlsMasterPlaylist(smoothStream, hasKeys)
	if err != nil {
		http.Error(w, "Error transforming manifest", http.StatusInternalServerError)
		log.Printf("Error transforming manifest: %v", err)
		return
	}
	manifestTransformTook := time.Since(manifestTransformStartTime)

	reqStartTime := r.Context().Value("reqStartTime").(time.Time)
	reqTook := time.Since(reqStartTime)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Content-Length", strconv.Itoa(len(playlist)))
	w.Header().Set("Server-Timing", fmt.Sprintf(
		"manifest-fetch;dur=%.3f,manifest-transform;dur=%.3f,total;dur=%.3f",
		manifestFetchTook.Seconds()*1000,
		manifestTransformTook.Seconds()*1000,
		reqTook.Seconds()*1000,
	))
	w.WriteHeader(http.StatusOK)

	w.Write(playlist)
}

// HlsMediaHandler handles requests for HLS media playlists (playlist.m3u8).
// It generates a media playlist for a specific quality level of a stream,
// listing all MPEG-TS segment URLs with their durations.
//
// The handler expects the following URL parameters:
//   - qualityId: The ID of the quality level (e.g., "video_0", "audio_eng_0").
//
// Segments are served as MPEG-TS (.ts) by HlsSegmentHandler.
func HlsMediaHandler(w http.ResponseWriter, r *http.Request) {
	channel, ok := r.Context().Value("channel").(config.Channel)
	if !ok {
		http.Error(w, "Channel not found in context", http.StatusInternalServerError)
		return
	}

	qualityId := r.PathValue("qualityId")

	// split from right, because we can have audio_deu_0 where we want audio_deu and 0
	lastUnderscore := strings.LastIndex(qualityId, "_")
	if lastUnderscore == -1 || lastUnderscore == len(qualityId)-1 {
		http.Error(w, "Invalid quality ID format", http.StatusBadRequest)
		return
	}

	streamIndexStr := qualityId[:lastUnderscore]
	qualityLevelIndexStr := qualityId[lastUnderscore+1:]
	qualityLevelIndex, err := strconv.Atoi(qualityLevelIndexStr)
	if err != nil {
		http.Error(w, "Invalid quality level index", http.StatusBadRequest)
		return
	}

	manifestFetchStartTime := time.Now()
	smoothStream, err := transformers.GetSmoothManifest(channel.Url)
	if err != nil {
		http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
		log.Printf("Error fetching manifest: %v", err)
		return
	}
	manifestFetchTook := time.Since(manifestFetchStartTime)

	manifestTransformStartTime := time.Now()
	playlist, err := transformers.SmoothToHlsMediaPlaylist(smoothStream, streamIndexStr, qualityLevelIndex)
	if err != nil {
		http.Error(w, "Error generating media playlist", http.StatusInternalServerError)
		log.Printf("Error generating media playlist: %v", err)
		return
	}
	manifestTransformTook := time.Since(manifestTransformStartTime)

	reqStartTime := r.Context().Value("reqStartTime").(time.Time)
	reqTook := time.Since(reqStartTime)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Content-Length", strconv.Itoa(len(playlist)))
	w.Header().Set("Server-Timing", fmt.Sprintf(
		"manifest-fetch;dur=%.3f,manifest-transform;dur=%.3f,total;dur=%.3f",
		manifestFetchTook.Seconds()*1000,
		manifestTransformTook.Seconds()*1000,
		reqTook.Seconds()*1000,
	))
	w.WriteHeader(http.StatusOK)

	w.Write(playlist)
}
