package srv

import (
	"net/http"
	"strconv"

	qrcode "github.com/skip2/go-qrcode"
)

func handleQRCode(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "url parameter is required", http.StatusBadRequest)
		return
	}

	size := 256
	if s := r.URL.Query().Get("size"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			size = n
		}
	}
	if size > 1024 {
		size = 1024
	}

	png, err := qrcode.Encode(rawURL, qrcode.Medium, size)
	if err != nil {
		http.Error(w, "failed to generate QR code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(png)
}
