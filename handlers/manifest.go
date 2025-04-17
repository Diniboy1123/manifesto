package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/transformers"
)

// DashManifestHandler seamlessly transforms a SmoothStream manifest to a DASH manifest.
// It handles the request, fetches the SmoothStream manifest, transforms it to DASH,
// and writes the DASH manifest to the response.
//
// The handler expects the channel information to be present in the request context.
// If the channel is not found in the context, it returns an error response.
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

	smoothStream, err := transformers.GetSmoothManifest(channel.Url)
	if err != nil {
		http.Error(w, "Error fetching manifest", http.StatusInternalServerError)
		log.Printf("Error fetching manifest: %v", err)
		return
	}

	var hasKeys bool
	if channel.Keys != nil {
		hasKeys = true
	}

	mpd, err := transformers.SmoothToDashManifest(smoothStream, hasKeys, config.Get().AllowSubs, channel)
	if err != nil {
		http.Error(w, "Error transforming manifest", http.StatusInternalServerError)
		log.Printf("Error transforming manifest: %v", err)
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
