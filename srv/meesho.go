package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"srv.exe.dev/db/dbgen"
)

// MeeshoProduct represents a product scraped from Meesho
type MeeshoProduct struct {
	MeeshoID    int64    `json:"meesho_id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	OrigSlug    string   `json:"original_slug"`
	Price       int64    `json:"price"`
	CatalogPrice int64   `json:"catalog_price"`
	Description string   `json:"description"`
	Image       string   `json:"image"`
	Images      []string `json:"images"`
	Category    string   `json:"category"`
	Rating      string   `json:"rating"`
	RatingCount int64    `json:"rating_count"`
	URL         string   `json:"url"`
}

// BulkImportStatus tracks the progress of a bulk import
type BulkImportStatus struct {
	mu          sync.Mutex
	Running     bool             `json:"running"`
	Total       int              `json:"total"`
	Imported    int              `json:"imported"`
	Skipped     int              `json:"skipped"`
	Failed      int              `json:"failed"`
	Errors      []string         `json:"errors"`
	Products    []MeeshoProduct  `json:"products,omitempty"`
	Message     string           `json:"message"`
	StartedAt   time.Time        `json:"started_at"`
	FinishedAt  *time.Time       `json:"finished_at,omitempty"`
}

var bulkImportStatus = &BulkImportStatus{}

// scrapeMeeshoStorePage fetches a Meesho supplier page and extracts products from __NEXT_DATA__
func scrapeMeeshoStorePage(storeURL string) ([]MeeshoProduct, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", storeURL, nil)
	if err != nil {
		return nil, 0, err
	}
	// Mobile UA works better with Meesho
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, 0, fmt.Errorf("Meesho blocked the request (403). Try again in a few minutes")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, 0, err
	}

	return parseMeeshoPage(string(body))
}

// parseMeeshoPage extracts products and total count from Meesho HTML
func parseMeeshoPage(html string) ([]MeeshoProduct, int, error) {
	// Find __NEXT_DATA__ JSON
	re := regexp.MustCompile(`__NEXT_DATA__[^>]*type="application/json">(.*?)</script>`)
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		if strings.Contains(html, "Access Denied") {
			return nil, 0, fmt.Errorf("Meesho blocked the request. Try again in a few minutes")
		}
		return nil, 0, fmt.Errorf("could not find product data on page")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, 0, fmt.Errorf("failed to parse page data: %w", err)
	}

	// Navigate: props.pageProps.initialState.shopListing.listing
	listing, err := navigateJSON(data, "props", "pageProps", "initialState", "shopListing", "listing")
	if err != nil {
		// Try alternative path for category pages
		listing, err = navigateJSON(data, "props", "pageProps", "initialState", "hpListing", "listing")
		if err != nil {
			return nil, 0, fmt.Errorf("could not find listing data")
		}
	}

	listingMap, ok := listing.(map[string]interface{})
	if !ok {
		return nil, 0, fmt.Errorf("unexpected listing format")
	}

	totalCount := 0
	if tc, ok := listingMap["productsCount"].(float64); ok {
		totalCount = int(tc)
	}

	// Get products array
	pages, ok := listingMap["products"].([]interface{})
	if !ok || len(pages) == 0 {
		return nil, totalCount, fmt.Errorf("no products found")
	}

	var products []MeeshoProduct
	for _, page := range pages {
		pageMap, ok := page.(map[string]interface{})
		if !ok {
			continue
		}
		prods, ok := pageMap["products"].([]interface{})
		if !ok {
			continue
		}
		for _, prod := range prods {
			p, ok := prod.(map[string]interface{})
			if !ok {
				continue
			}
			mp := parseMeeshoProduct(p)
			products = append(products, mp)
		}
	}

	return products, totalCount, nil
}

func parseMeeshoProduct(p map[string]interface{}) MeeshoProduct {
	mp := MeeshoProduct{}

	if v, ok := p["id"].(float64); ok {
		mp.MeeshoID = int64(v)
	}
	if v, ok := p["name"].(string); ok {
		mp.Name = strings.TrimSpace(v)
	}
	if v, ok := p["slug"].(string); ok {
		mp.Slug = v
	}
	if v, ok := p["original_slug"].(string); ok {
		mp.OrigSlug = v
	}
	if v, ok := p["min_product_price"].(float64); ok {
		mp.Price = int64(v)
	}
	if v, ok := p["min_catalog_price"].(float64); ok {
		mp.CatalogPrice = int64(v)
	}
	if v, ok := p["description"].(string); ok {
		mp.Description = strings.TrimSpace(v)
	}
	if mp.Description == "" {
		mp.Description = mp.Name
	}
	if v, ok := p["image"].(string); ok {
		mp.Image = v
	}
	if v, ok := p["sub_sub_category_name"].(string); ok {
		mp.Category = v
	}

	// Images
	if imgs, ok := p["images"].([]interface{}); ok && len(imgs) > 0 {
		for _, img := range imgs {
			if s, ok := img.(string); ok {
				mp.Images = append(mp.Images, s)
			}
		}
	}
	if len(mp.Images) == 0 {
		if pis, ok := p["product_images"].([]interface{}); ok {
			for _, pi := range pis {
				if piMap, ok := pi.(map[string]interface{}); ok {
					if url, ok := piMap["url"].(string); ok {
						mp.Images = append(mp.Images, url)
					}
				}
			}
		}
	}

	// Rating
	if rs, ok := p["supplier_reviews_summary"].(map[string]interface{}); ok {
		if v, ok := rs["average_rating_str"].(string); ok {
			mp.Rating = v
		}
		if v, ok := rs["rating_count"].(float64); ok {
			mp.RatingCount = int64(v)
		}
	}

	// Build URL
	if mp.OrigSlug != "" {
		mp.URL = fmt.Sprintf("https://www.meesho.com/%s/p/%s", mp.Slug, mp.OrigSlug)
	} else if mp.Slug != "" {
		mp.URL = fmt.Sprintf("https://www.meesho.com/%s", mp.Slug)
	}

	return mp
}

func navigateJSON(data interface{}, keys ...string) (interface{}, error) {
	current := data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map at key %s", key)
		}
		current, ok = m[key]
		if !ok {
			return nil, fmt.Errorf("key %s not found", key)
		}
	}
	return current, nil
}

// mapMeeshoCategory maps Meesho subcategory names to our categories
func mapMeeshoCategory(meeshoCat string) string {
	l := strings.ToLower(meeshoCat)
	switch {
	case strings.Contains(l, "nail"):
		return "Nails & Beauty"
	case strings.Contains(l, "cap") || strings.Contains(l, "hat") || strings.Contains(l, "beanie") || strings.Contains(l, "accessories"):
		return "Caps & Accessories"
	case strings.Contains(l, "sweatshirt") || strings.Contains(l, "hoodie") || strings.Contains(l, "shirt") || strings.Contains(l, "kurti") || strings.Contains(l, "dress") || strings.Contains(l, "fashion"):
		return "Fashion & Clothing"
	case strings.Contains(l, "lamp") || strings.Contains(l, "light") || strings.Contains(l, "led") || strings.Contains(l, "decor") || strings.Contains(l, "home"):
		return "Home & Decor"
	case strings.Contains(l, "bottle") || strings.Contains(l, "jar") || strings.Contains(l, "mug") || strings.Contains(l, "cup") || strings.Contains(l, "tumbler") || strings.Contains(l, "sipper") || strings.Contains(l, "kitchen"):
		return "Kitchen & Dining"
	default:
		return autoCategory(meeshoCat)
	}
}

// handleBulkImport starts a bulk import from a Meesho store
func (s *Server) handleBulkImport(w http.ResponseWriter, r *http.Request) {
	storeURL := r.FormValue("store_url")
	if storeURL == "" {
		storeURL = "https://www.meesho.com/ShuKarshEnterprises"
	}

	// Normalize URL
	if !strings.HasPrefix(storeURL, "http") {
		storeURL = "https://www.meesho.com/" + storeURL
	}

	bulkImportStatus.mu.Lock()
	if bulkImportStatus.Running {
		bulkImportStatus.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "Import already running"})
		return
	}
	bulkImportStatus.Running = true
	bulkImportStatus.Total = 0
	bulkImportStatus.Imported = 0
	bulkImportStatus.Skipped = 0
	bulkImportStatus.Failed = 0
	bulkImportStatus.Errors = nil
	bulkImportStatus.Products = nil
	bulkImportStatus.Message = "Starting import..."
	bulkImportStatus.StartedAt = time.Now()
	bulkImportStatus.FinishedAt = nil
	bulkImportStatus.mu.Unlock()

	go s.runBulkImport(storeURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "Import started"})
}

func (s *Server) runBulkImport(storeURL string) {
	defer func() {
		bulkImportStatus.mu.Lock()
		bulkImportStatus.Running = false
		now := time.Now()
		bulkImportStatus.FinishedAt = &now
		bulkImportStatus.mu.Unlock()
	}()

	bulkImportStatus.mu.Lock()
	bulkImportStatus.Message = "Fetching store page..."
	bulkImportStatus.mu.Unlock()

	products, totalCount, err := scrapeMeeshoStorePage(storeURL)
	if err != nil {
		bulkImportStatus.mu.Lock()
		bulkImportStatus.Message = "Error: " + err.Error()
		bulkImportStatus.Errors = append(bulkImportStatus.Errors, err.Error())
		bulkImportStatus.mu.Unlock()
		return
	}

	bulkImportStatus.mu.Lock()
	bulkImportStatus.Total = len(products)
	bulkImportStatus.Message = fmt.Sprintf("Found %d products (of %d total in store). Importing...", len(products), totalCount)
	bulkImportStatus.mu.Unlock()

	// Get existing product URLs to avoid duplicates
	q := dbgen.New(s.DB)
	ctx := context.Background()
	existing, _ := q.ListProducts(ctx)
	existingTitles := make(map[string]bool)
	for _, p := range existing {
		existingTitles[strings.ToLower(strings.TrimSpace(p.Title))] = true
	}

	for i, mp := range products {
		bulkImportStatus.mu.Lock()
		bulkImportStatus.Message = fmt.Sprintf("Processing %d/%d: %s", i+1, len(products), mp.Name)
		bulkImportStatus.mu.Unlock()

		// Skip if already exists
		if existingTitles[strings.ToLower(strings.TrimSpace(mp.Name))] {
			bulkImportStatus.mu.Lock()
			bulkImportStatus.Skipped++
			bulkImportStatus.mu.Unlock()
			continue
		}

		// Map category
		category := mapMeeshoCategory(mp.Category)

		// Build images JSON
		imagesJSON := "[]"
		if len(mp.Images) > 0 {
			b, _ := json.Marshal(mp.Images)
			imagesJSON = string(b)
		}

		// Main image
		imageURL := mp.Image
		if imageURL == "" && len(mp.Images) > 0 {
			imageURL = mp.Images[0]
		}

		// Price
		priceStr := fmt.Sprintf("\u20b9%d", mp.Price)
		origPriceStr := ""
		if mp.CatalogPrice > mp.Price {
			origPriceStr = fmt.Sprintf("\u20b9%d", mp.CatalogPrice)
		}

		_, err := q.InsertProduct(ctx, dbgen.InsertProductParams{
			Url:           mp.URL,
			Platform:      "Meesho",
			Title:         mp.Name,
			Price:         priceStr,
			OriginalPrice: origPriceStr,
			ImageUrl:      imageURL,
			Description:   mp.Description,
			Rating:        mp.Rating,
			Category:      category,
			Images:        imagesJSON,
		})
		if err != nil {
			bulkImportStatus.mu.Lock()
			bulkImportStatus.Failed++
			bulkImportStatus.Errors = append(bulkImportStatus.Errors, fmt.Sprintf("%s: %s", mp.Name, err.Error()))
			bulkImportStatus.mu.Unlock()
			continue
		}

		bulkImportStatus.mu.Lock()
		bulkImportStatus.Imported++
		bulkImportStatus.Products = append(bulkImportStatus.Products, mp)
		existingTitles[strings.ToLower(strings.TrimSpace(mp.Name))] = true
		bulkImportStatus.mu.Unlock()
	}

	bulkImportStatus.mu.Lock()
	bulkImportStatus.Message = fmt.Sprintf("Done! Imported %d, skipped %d (already exist), failed %d",
		bulkImportStatus.Imported, bulkImportStatus.Skipped, bulkImportStatus.Failed)
	bulkImportStatus.mu.Unlock()
}

// handleBulkImportStatus returns the current import status
func (s *Server) handleBulkImportStatus(w http.ResponseWriter, r *http.Request) {
	bulkImportStatus.mu.Lock()
	defer bulkImportStatus.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bulkImportStatus)
}

// handleBulkImportJSON imports products from a JSON payload (for manual paste of scraped data)
func (s *Server) handleBulkImportJSON(w http.ResponseWriter, r *http.Request) {
	var products []MeeshoProduct
	if err := json.NewDecoder(r.Body).Decode(&products); err != nil {
		jsonError(w, "Invalid JSON: "+err.Error(), 400)
		return
	}

	q := dbgen.New(s.DB)
	existing, _ := q.ListProducts(r.Context())
	existingTitles := make(map[string]bool)
	for _, p := range existing {
		existingTitles[strings.ToLower(strings.TrimSpace(p.Title))] = true
	}

	imported, skipped := 0, 0
	for _, mp := range products {
		if existingTitles[strings.ToLower(strings.TrimSpace(mp.Name))] {
			skipped++
			continue
		}

		category := mapMeeshoCategory(mp.Category)
		imagesJSON := "[]"
		if len(mp.Images) > 0 {
			b, _ := json.Marshal(mp.Images)
			imagesJSON = string(b)
		}
		imageURL := mp.Image
		if imageURL == "" && len(mp.Images) > 0 {
			imageURL = mp.Images[0]
		}
		priceStr := fmt.Sprintf("₹%d", mp.Price)
		origPriceStr := ""
		if mp.CatalogPrice > mp.Price {
			origPriceStr = fmt.Sprintf("₹%d", mp.CatalogPrice)
		}

		_, err := q.InsertProduct(r.Context(), dbgen.InsertProductParams{
			Url:           mp.URL,
			Platform:      "Meesho",
			Title:         mp.Name,
			Price:         priceStr,
			OriginalPrice: origPriceStr,
			ImageUrl:      imageURL,
			Description:   mp.Description,
			Rating:        mp.Rating,
			Category:      category,
			Images:        imagesJSON,
		})
		if err != nil {
			continue
		}
		imported++
		existingTitles[strings.ToLower(strings.TrimSpace(mp.Name))] = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"imported": imported,
		"skipped":  skipped,
		"total":    len(products),
	})
}
