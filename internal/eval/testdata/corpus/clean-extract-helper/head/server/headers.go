package server

import (
	"net/http"
	"strings"
)

// IsJSON reports whether the request body is JSON.
func IsJSON(r *http.Request) bool {
	return mediaType(r) == "application/json"
}

// IsForm reports whether the request body is a URL-encoded form.
func IsForm(r *http.Request) bool {
	return mediaType(r) == "application/x-www-form-urlencoded"
}

// mediaType returns the lower-case Content-Type without parameters.
func mediaType(r *http.Request) string {
	ct := r.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}
