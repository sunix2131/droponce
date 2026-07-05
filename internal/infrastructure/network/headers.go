package network

import "net/http"

func SecurityHeaders(w http.ResponseWriter, html bool) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, private")
	h.Set("Pragma", "no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	if html {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; manifest-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	}
}
