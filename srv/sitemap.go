package srv

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	// Detect the base URL from the request
	scheme := "https"
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	baseURL := scheme + "://" + host

	q := dbgen.New(s.DB)
	products, err := q.ListProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sb.WriteString("\n")

	// Homepage
	now := time.Now().Format("2006-01-02")
	sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s/</loc>
    <lastmod>%s</lastmod>
    <changefreq>daily</changefreq>
    <priority>1.0</priority>
  </url>
`, baseURL, now))

	// Product pages
	for _, p := range products {
		addedAt := now
		if !p.AddedAt.IsZero() {
			addedAt = p.AddedAt.Format("2006-01-02")
		}
		sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s/product/%d</loc>
    <lastmod>%s</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
`, baseURL, p.ID, addedAt))
	}

	// Category pages via search
	categories := []string{"Nails & Beauty", "Caps & Accessories", "Fashion & Clothing", "Home & Decor", "Kitchen & Dining"}
	for _, cat := range categories {
		hasProducts := false
		for _, p := range products {
			if p.Category == cat {
				hasProducts = true
				break
			}
		}
		if hasProducts {
			sb.WriteString(fmt.Sprintf(`  <url>
    <loc>%s/search?q=%s</loc>
    <changefreq>weekly</changefreq>
    <priority>0.6</priority>
  </url>
`, baseURL, cat))
		}
	}

	sb.WriteString(`</urlset>`)
	fmt.Fprint(w, sb.String())
}

func (s *Server) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	baseURL := scheme + "://" + host

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, `User-agent: *
Allow: /
Disallow: /admin
Disallow: /api/
Disallow: /img

Sitemap: %s/sitemap.xml
`, baseURL)
}


