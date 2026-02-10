package srv

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type ProductInfo struct {
	URL           string
	Platform      string
	Title         string
	Price         string
	OriginalPrice string
	ImageURL      string
	Description   string
	Rating        string
}

func ScrapeProduct(rawURL string) (*ProductInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("empty URL")
	}

	platform := detectPlatform(rawURL)
	if platform == "" {
		return nil, fmt.Errorf("unsupported platform — use Meesho or Amazon URLs")
	}

	body, err := fetchPage(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}

	p := &ProductInfo{URL: rawURL, Platform: platform}

	// Extract from meta tags (og: tags, standard meta)
	p.Title = firstNonEmpty(
		extractMeta(body, `og:title`),
		extractMeta(body, `twitter:title`),
		extractTag(body, `<title>`, `</title>`),
	)
	p.Description = firstNonEmpty(
		extractMeta(body, `og:description`),
		extractMeta(body, `description`),
		extractMeta(body, `twitter:description`),
	)
	p.ImageURL = firstNonEmpty(
		extractMeta(body, `og:image`),
		extractMeta(body, `twitter:image`),
	)

	// Price extraction
	p.Price = firstNonEmpty(
		extractMeta(body, `og:price:amount`),
		extractMeta(body, `product:price:amount`),
		extractPriceFromBody(body, platform),
	)
	p.OriginalPrice = firstNonEmpty(
		extractMeta(body, `product:original_price:amount`),
	)
	p.Rating = extractRating(body)

	// Clean up title (remove site names)
	p.Title = cleanTitle(p.Title, platform)

	if p.Title == "" {
		p.Title = "Product from " + platform
	}

	return p, nil
}

func detectPlatform(url string) string {
	l := strings.ToLower(url)
	switch {
	case strings.Contains(l, "meesho.com"):
		return "Meesho"
	case strings.Contains(l, "amazon.in"), strings.Contains(l, "amazon.com"),
		strings.Contains(l, "amzn.in"), strings.Contains(l, "amzn.to"),
		strings.Contains(l, "amzn.eu"):
		return "Amazon"
	case strings.Contains(l, "flipkart.com"), strings.Contains(l, "fkrt.it"):
		return "Flipkart"
	default:
		return "Other"
	}
}

func fetchPage(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,hi;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var metaPatterns = []*regexp.Regexp{
	regexp.MustCompile(`<meta[^>]+property=["']([^"']+)["'][^>]+content=["']([^"']*)["']`),
	regexp.MustCompile(`<meta[^>]+content=["']([^"']*)["'][^>]+property=["']([^"']+)["']`),
	regexp.MustCompile(`<meta[^>]+name=["']([^"']+)["'][^>]+content=["']([^"']*)["']`),
	regexp.MustCompile(`<meta[^>]+content=["']([^"']*)["'][^>]+name=["']([^"']+)["']`),
}

func extractMeta(body, prop string) string {
	propLower := strings.ToLower(prop)
	// pattern 1 & 3: property/name first, content second
	for _, i := range []int{0, 2} {
		for _, m := range metaPatterns[i].FindAllStringSubmatch(body, -1) {
			if strings.ToLower(m[1]) == propLower {
				return strings.TrimSpace(htmlDecode(m[2]))
			}
		}
	}
	// pattern 2 & 4: content first, property/name second
	for _, i := range []int{1, 3} {
		for _, m := range metaPatterns[i].FindAllStringSubmatch(body, -1) {
			if strings.ToLower(m[2]) == propLower {
				return strings.TrimSpace(htmlDecode(m[1]))
			}
		}
	}
	return ""
}

func extractTag(body, open, close string) string {
	i := strings.Index(body, open)
	if i < 0 {
		return ""
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(htmlDecode(rest[:j]))
}

var priceRe = regexp.MustCompile(`[₹$]\s*([\d,]+\.?\d*)`)

func extractPriceFromBody(body, platform string) string {
	// Look for JSON-LD price
	jsonPrice := regexp.MustCompile(`"price"\s*:\s*"?([\d,.]+)"?`)
	if m := jsonPrice.FindStringSubmatch(body); len(m) > 1 {
		return "₹" + m[1]
	}
	if m := priceRe.FindStringSubmatch(body); len(m) > 1 {
		return "₹" + m[1]
	}
	return ""
}

var ratingRe = regexp.MustCompile(`([\d.]+)\s*(?:out of|/)\s*5`)

func extractRating(body string) string {
	if m := ratingRe.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	// JSON-LD
	jsonRating := regexp.MustCompile(`"ratingValue"\s*:\s*"?([\d.]+)"?`)
	if m := jsonRating.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	return ""
}

func cleanTitle(title, platform string) string {
	// Remove common suffixes
	for _, s := range []string{
		" | Meesho", " - Meesho", ": Buy Online",
		" | Amazon.in", " - Amazon.in", ": Amazon.in",
		" | Flipkart", " - Flipkart.com",
		"Amazon.in:", "Amazon.in :",
	} {
		title = strings.ReplaceAll(title, s, "")
	}
	return strings.TrimSpace(title)
}

func htmlDecode(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&#x27;", "'", "&nbsp;", " ")
	return r.Replace(s)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
