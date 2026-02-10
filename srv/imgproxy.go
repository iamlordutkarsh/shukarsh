package srv

import (
	"io"
	"net/http"
	"strings"
	"time"
)

var proxyClient = &http.Client{Timeout: 10 * time.Second}

func handleImageProxy(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "missing url", 400)
		return
	}
	// Only proxy known CDN domains
	allowed := false
	for _, d := range []string{"images.meesho.com", "m.media-amazon.com", "rukminim", "img.fkcdn"} {
		if strings.Contains(rawURL, d) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "domain not allowed", 403)
		return
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.meesho.com/")
	req.Header.Set("Accept", "image/*,*/*")

	resp, err := proxyClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, resp.Body)
}
