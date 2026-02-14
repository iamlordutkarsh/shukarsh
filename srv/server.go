package srv

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname       string
	TemplatesDir   string
	StaticDir      string
	UploadsDir     string
	AdminPassword  string
	adminTokenHash [32]byte
}

func New(dbPath, hostname, adminPassword string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	uploadsDir := filepath.Join(filepath.Dir(baseDir), "uploads")
	if d := os.Getenv("UPLOADS_DIR"); d != "" {
		uploadsDir = d
	}
	os.MkdirAll(uploadsDir, 0755)
	srv := &Server{
		Hostname:      hostname,
		TemplatesDir:  filepath.Join(baseDir, "templates"),
		StaticDir:     filepath.Join(baseDir, "static"),
		UploadsDir:    uploadsDir,
		AdminPassword: adminPassword,
	}
	// Generate a stable session token from the password
	srv.adminTokenHash = sha256.Sum256([]byte("shukarsh-admin-" + adminPassword))
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	return srv, nil
}

func (s *Server) adminToken() string {
	return hex.EncodeToString(s.adminTokenHash[:16])
}

func (s *Server) isAdminAuthed(r *http.Request) bool {
	if s.AdminPassword == "" {
		return true // no password set, open access
	}
	c, err := r.Cookie("admin_token")
	if err != nil {
		return false
	}
	return c.Value == s.adminToken()
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminAuthed(r) {
			if r.Header.Get("Content-Type") == "application/json" || strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	return nil
}

func (s *Server) trackView(r *http.Request, productID *int64) {
	q := dbgen.New(s.DB)
	visitorID := ""
	if c, err := r.Cookie("vid"); err == nil {
		visitorID = c.Value
	}
	q.InsertPageView(r.Context(), dbgen.InsertPageViewParams{
		Path:      r.URL.Path,
		ProductID: productID,
		Referrer:  r.Referer(),
		UserAgent: r.UserAgent(),
		VisitorID: visitorID,
	})
}

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /product/{id}", s.handleProductDetail)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /category/{name}", s.handleCategory)
	mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdmin))
	mux.HandleFunc("GET /admin/analytics", s.requireAdmin(s.handleAnalytics))
	mux.HandleFunc("POST /api/wa-click", s.handleWAClick)
	mux.HandleFunc("GET /admin/login", s.handleAdminLogin)
	mux.HandleFunc("POST /admin/login", s.handleAdminLoginPost)
	mux.HandleFunc("GET /admin/logout", s.handleAdminLogout)
	mux.HandleFunc("POST /api/add", s.requireAdmin(s.handleAddProduct))
	mux.HandleFunc("POST /api/update/{id}", s.requireAdmin(s.handleUpdateProduct))
	mux.HandleFunc("POST /api/delete/{id}", s.requireAdmin(s.handleDeleteProduct))
	mux.HandleFunc("GET /api/products", s.handleListProducts)
	mux.HandleFunc("GET /api/product/{id}", s.handleGetProduct)
	mux.HandleFunc("GET /img", handleImageProxy)
	mux.HandleFunc("GET /api/qr", handleQRCode)
	mux.HandleFunc("POST /api/upload", s.requireAdmin(s.handleUploadImage))
	mux.HandleFunc("POST /api/bulk-import", s.requireAdmin(s.handleBulkImport))
	mux.HandleFunc("GET /api/bulk-import/status", s.handleBulkImportStatus)
	mux.HandleFunc("POST /api/bulk-import/json", s.requireAdmin(s.handleBulkImportJSON))
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /robots.txt", s.handleRobotsTxt)
	mux.HandleFunc("GET /ads.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.StaticDir, "ads.txt"))
	})
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.UploadsDir))))
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// parsePrice extracts a numeric price from strings like "₹370", "Rs. 1,234", etc.
func parsePrice(s string) float64 {
	var buf strings.Builder
	for _, c := range s {
		if (c >= '0' && c <= '9') || c == '.' {
			buf.WriteRune(c)
		}
	}
	v, _ := strconv.ParseFloat(buf.String(), 64)
	return v
}

var funcMap = template.FuncMap{
	"lower": strings.ToLower,
	"mul": func(a, b int) int { return a * b },
	"discountPct": func(price, origPrice string) int {
		p := parsePrice(price)
		o := parsePrice(origPrice)
		if o <= 0 || p <= 0 || o <= p {
			return 0
		}
		return int(((o - p) / o) * 100)
	},
	"catEmoji": func(cat string) string {
		switch cat {
		case "Nails & Beauty":
			return "💅"
		case "Caps & Accessories":
			return "🧢"
		case "Fashion & Clothing":
			return "👗"
		case "Home & Decor":
			return "🏠"
		case "Kitchen & Dining":
			return "🍽️"
		case "Electronics":
			return "📱"
		default:
			return "📦"
		}
	},
	"catGif": func(cat string) string {
		base := "https://fonts.gstatic.com/s/e/notoemoji/latest/"
		switch cat {
		case "Nails & Beauty":
			return base + "1f485/512.gif"
		case "Caps & Accessories":
			return base + "1f48e/512.gif"
		case "Fashion & Clothing":
			return base + "1f49c/512.gif"
		case "Home & Decor":
			return base + "1f4a1/512.gif"
		case "Kitchen & Dining":
			return base + "2615/512.gif"
		case "Electronics":
			return base + "1f4ab/512.gif"
		default:
			return base + "1f381/512.gif"
		}
	},
	"catCount": func(m map[string][]dbgen.Product, cat string) int {
		return len(m[cat])
	},
	"splitImages": func(s string) []string {
		if s == "" {
			return nil
		}
		var imgs []string
		json.Unmarshal([]byte(s), &imgs)
		return imgs
	},
	"json": func(v any) string {
		b, _ := json.Marshal(v)
		return string(b)
	},
	"fmtPrice": func(price string) string {
		if price == "" {
			return ""
		}
		p := strings.TrimSpace(price)
		if strings.HasPrefix(p, "₹") || strings.HasPrefix(p, "Rs") {
			return p
		}
		return "₹" + p
	},
	"imgSrc": func(url string) string {
		if strings.HasPrefix(url, "/uploads/") || strings.HasPrefix(url, "/static/") {
			return url
		}
		return "/img?url=" + url
	},
	"add": func(a, b int) int { return a + b },
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "..."
	},
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	go s.trackView(r, nil)
	q := dbgen.New(s.DB)
	products, err := q.ListProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	catMap := map[string][]dbgen.Product{}
	catOrder := []string{}
	for _, p := range products {
		cat := p.Category
		if cat == "" {
			cat = "Other"
		}
		if _, ok := catMap[cat]; !ok {
			catOrder = append(catOrder, cat)
		}
		catMap[cat] = append(catMap[cat], p)
	}
	newArrivals, _ := q.ListNewArrivals(r.Context())
	bestSellers, _ := q.ListBestSellers(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("home.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "home.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Build featured products for hero carousel (bestsellers + new arrivals, deduplicated)
	featuredMap := map[int64]bool{}
	var featured []dbgen.Product
	for _, p := range bestSellers {
		if !featuredMap[p.ID] {
			featuredMap[p.ID] = true
			featured = append(featured, p)
		}
	}
	for _, p := range newArrivals {
		if !featuredMap[p.ID] {
			featuredMap[p.ID] = true
			featured = append(featured, p)
		}
	}
	// If fewer than 4 featured, pad with recent products
	for _, p := range products {
		if len(featured) >= 5 {
			break
		}
		if !featuredMap[p.ID] {
			featuredMap[p.ID] = true
			featured = append(featured, p)
		}
	}
	// Cap at 5 featured products (6 slides total with welcome)
	if len(featured) > 5 {
		featured = featured[:5]
	}

	// Dynamic stats
	totalViews, _ := q.TotalViews(r.Context())
	uniqueVisitors, _ := q.UniqueVisitors(r.Context())
	waClicks, _ := q.TotalWAClicks(r.Context())

	tmpl.Execute(w, map[string]any{
		"Products":       products,
		"Categories":     catOrder,
		"ByCategory":     catMap,
		"NewArrivals":    newArrivals,
		"BestSellers":    bestSellers,
		"Featured":       featured,
		"TotalViews":     totalViews,
		"UniqueVisitors": uniqueVisitors,
		"WAClicks":       waClicks,
	})
}

func (s *Server) handleProductDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", 400)
		return
	}
	go s.trackView(r, &id)
	q := dbgen.New(s.DB)
	product, err := q.GetProduct(r.Context(), id)
	if err != nil {
		http.Error(w, "Product not found", 404)
		return
	}

	// Get related products from same category
	var related []dbgen.Product
	if product.Category != "" {
		related, _ = q.ListProductsByCategory(r.Context(), product.Category)
	}
	// Filter out current product and limit to 4
	var filteredRelated []dbgen.Product
	for _, rp := range related {
		if rp.ID != product.ID {
			filteredRelated = append(filteredRelated, rp)
		}
		if len(filteredRelated) >= 4 {
			break
		}
	}

	// Parse images JSON
	var images []string
	if product.Images != "" {
		json.Unmarshal([]byte(product.Images), &images)
	}
	if len(images) == 0 && product.ImageUrl != "" {
		images = []string{product.ImageUrl}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("product.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "product.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{
		"Product":  product,
		"Images":   images,
		"Related":  filteredRelated,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	go s.trackView(r, nil)
	query := r.URL.Query().Get("q")
	var products []dbgen.Product
	if query != "" {
		q := dbgen.New(s.DB)
		like := "%" + query + "%"
		products, _ = q.SearchProducts(r.Context(), dbgen.SearchProductsParams{
			Title:       like,
			Description: like,
			Category:    like,
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("search.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "search.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{
		"Query":    query,
		"Products": products,
		"Count":    len(products),
	})
}

func (s *Server) handleCategory(w http.ResponseWriter, r *http.Request) {
	go s.trackView(r, nil)
	catName := r.PathValue("name")
	q := dbgen.New(s.DB)
	products, _ := q.ListProductsByCategory(r.Context(), catName)
	categories, _ := q.ListCategories(r.Context())

	sort := r.URL.Query().Get("sort")
	switch sort {
	case "price-asc":
		slices.SortFunc(products, func(a, b dbgen.Product) int {
			return cmp.Compare(parsePrice(a.Price), parsePrice(b.Price))
		})
	case "price-desc":
		slices.SortFunc(products, func(a, b dbgen.Product) int {
			return cmp.Compare(parsePrice(b.Price), parsePrice(a.Price))
		})
	case "newest":
		// already sorted by added_at DESC from query
	case "bestseller":
		slices.SortFunc(products, func(a, b dbgen.Product) int {
			return cmp.Compare(b.IsBestseller, a.IsBestseller)
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("category.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "category.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{
		"Category":   catName,
		"Products":   products,
		"Count":      len(products),
		"Sort":       sort,
		"Categories": categories,
	})
}

func (s *Server) handleWAClick(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	var pid *int64
	if v := r.FormValue("product_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			pid = &id
		}
	}
	clickType := r.FormValue("type")
	if clickType == "" {
		clickType = "order"
	}
	q.InsertWAClick(r.Context(), dbgen.InsertWAClickParams{ProductID: pid, ClickType: clickType})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	viewsPerDay, _ := q.ViewsPerDay(r.Context())
	topProducts, _ := q.TopProducts(r.Context())
	totalViews, _ := q.TotalViews(r.Context())
	todayViews, _ := q.TodayViews(r.Context())
	totalWA, _ := q.TotalWAClicks(r.Context())
	todayWA, _ := q.TodayWAClicks(r.Context())
	uniqueVisitors, _ := q.UniqueVisitors(r.Context())
	waByType, _ := q.WAClicksByType(r.Context())
	productCount := 0
	if products, err := q.ListProducts(r.Context()); err == nil {
		productCount = len(products)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("analytics.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "analytics.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{
		"ViewsPerDay":    viewsPerDay,
		"TopProducts":    topProducts,
		"TotalViews":     totalViews,
		"TodayViews":     todayViews,
		"TotalWA":        totalWA,
		"TodayWA":        todayWA,
		"UniqueVisitors": uniqueVisitors,
		"WAByType":       waByType,
		"ProductCount":   productCount,
	})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if s.AdminPassword == "" || s.isAdminAuthed(r) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	errMsg := r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>Admin Login — Shukarsh</title>
<link href="https://fonts.googleapis.com/css2?family=DM+Serif+Display&family=Nunito:wght@400;600;700&family=Satisfy&display=swap" rel="stylesheet">
<style>
:root{--bg:#faf0e4;--lav:#c9b3e8;--lavd:#a78bca;--lavl:#e8ddf5;--text:#2c2137;--white:#fff}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Nunito',sans-serif;background:var(--bg);min-height:100vh;display:flex;align-items:center;justify-content:center}
.login-card{background:var(--white);border-radius:24px;padding:48px 40px;width:100%%;max-width:400px;box-shadow:0 8px 40px rgba(169,139,202,.15);text-align:center}
.login-card .logo{font-family:'Satisfy',cursive;font-size:2.4rem;color:var(--lavd);margin-bottom:4px}
.login-card .sub{color:#6b5e7b;font-size:.9rem;margin-bottom:32px}
.login-card .lock{font-size:3rem;margin-bottom:16px}
.field{position:relative;margin-bottom:20px}
.field input{width:100%%;padding:14px 18px;border:2px solid var(--lavl);border-radius:14px;font-size:1rem;font-family:'Nunito',sans-serif;outline:none;transition:border-color .3s}
.field input:focus{border-color:var(--lavd)}
.btn{width:100%%;padding:14px;background:var(--lavd);color:var(--white);border:none;border-radius:14px;font-size:1rem;font-weight:700;font-family:'Nunito',sans-serif;cursor:pointer;transition:all .3s;letter-spacing:.5px}
.btn:hover{background:#9673bf;transform:translateY(-2px);box-shadow:0 6px 20px rgba(169,139,202,.3)}
.error{background:#fce4ec;color:#c62828;padding:10px 16px;border-radius:10px;font-size:.85rem;margin-bottom:16px}
.back{display:inline-block;margin-top:20px;color:var(--lavd);font-size:.85rem;text-decoration:none;font-weight:600}
.back:hover{text-decoration:underline}
</style>
</head>
<body>
<div class="login-card">
  <div class="lock">🔐</div>
  <div class="logo">Shukarsh</div>
  <div class="sub">Admin Panel</div>
  %s
  <form method="POST" action="/admin/login">
    <div class="field">
      <input type="password" name="password" placeholder="Enter admin password" autofocus required>
    </div>
    <button type="submit" class="btn">Login →</button>
  </form>
  <a href="/" class="back">← Back to Store</a>
</div>
</body>
</html>`,
		func() string {
			if errMsg != "" {
				return `<div class="error">❌ ` + template.HTMLEscapeString(errMsg) + `</div>`
			}
			return ""
		}())
}

func (s *Server) handleAdminLoginPost(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if password != s.AdminPassword {
		http.Redirect(w, r, "/admin/login?error=Wrong+password", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    s.adminToken(),
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "admin_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	products, _ := q.ListProducts(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.ParseFiles(filepath.Join(s.TemplatesDir, "admin.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{"Products": products})
}

func (s *Server) handleAddProduct(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("mode")
	var params dbgen.InsertProductParams

	if mode == "manual" {
		params = dbgen.InsertProductParams{
			Url:             r.FormValue("url"),
			Platform:        r.FormValue("platform"),
			Title:           r.FormValue("title"),
			Price:           r.FormValue("price"),
			OriginalPrice:   r.FormValue("original_price"),
			ImageUrl:        r.FormValue("image_url"),
			Description:     r.FormValue("description"),
			Rating:          r.FormValue("rating"),
			Category:        r.FormValue("category"),
			Images:          r.FormValue("images"),
			LongDescription: r.FormValue("long_description"),
		}
		if params.Title == "" {
			jsonError(w, "Title is required", 400)
			return
		}
		if params.Platform == "" {
			params.Platform = detectPlatform(params.Url)
			if params.Platform == "" {
				params.Platform = "Other"
			}
		}
	} else {
		url := r.FormValue("url")
		if url == "" {
			jsonError(w, "URL is required", 400)
			return
		}
		info, err := ScrapeProduct(url)
		if err != nil {
			jsonError(w, "Failed to scrape: "+err.Error(), 400)
			return
		}
		params = dbgen.InsertProductParams{
			Url:           info.URL,
			Platform:      info.Platform,
			Title:         info.Title,
			Price:         info.Price,
			OriginalPrice: info.OriginalPrice,
			ImageUrl:      info.ImageURL,
			Description:   info.Description,
			Rating:        info.Rating,
			Category:      autoCategory(info.Title),
		}
	}

	q := dbgen.New(s.DB)
	p, err := q.InsertProduct(r.Context(), params)
	if err != nil {
		jsonError(w, "Failed to save: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": p})
}

func (s *Server) handleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, "Invalid ID", 400)
		return
	}
	q := dbgen.New(s.DB)
	// Get existing product first
	p, err := q.GetProduct(r.Context(), id)
	if err != nil {
		jsonError(w, "Product not found", 404)
		return
	}

	// Update only fields that are provided
	title := r.FormValue("title")
	if title == "" {
		title = p.Title
	}
	price := r.FormValue("price")
	if price == "" {
		price = p.Price
	}
	origPrice := r.FormValue("original_price")
	if origPrice == "" {
		origPrice = p.OriginalPrice
	}
	imageUrl := r.FormValue("image_url")
	if imageUrl == "" {
		imageUrl = p.ImageUrl
	}
	desc := r.FormValue("description")
	if desc == "" {
		desc = p.Description
	}
	rating := r.FormValue("rating")
	if rating == "" {
		rating = p.Rating
	}
	category := r.FormValue("category")
	if category == "" {
		category = p.Category
	}
	images := r.FormValue("images")
	if images == "" {
		images = p.Images
	}
	longDesc := r.FormValue("long_description")
	if longDesc == "" {
		longDesc = p.LongDescription
	}
	url := r.FormValue("url")
	if url == "" {
		url = p.Url
	}
	platform := r.FormValue("platform")
	if platform == "" {
		platform = p.Platform
	}

	isNew := p.IsNew
	if v := r.FormValue("is_new"); v != "" {
		isNew, _ = strconv.ParseInt(v, 10, 64)
	}
	isBestseller := p.IsBestseller
	if v := r.FormValue("is_bestseller"); v != "" {
		isBestseller, _ = strconv.ParseInt(v, 10, 64)
	}

	err = q.UpdateProduct(r.Context(), dbgen.UpdateProductParams{
		Title:           title,
		Price:           price,
		OriginalPrice:   origPrice,
		ImageUrl:        imageUrl,
		Description:     desc,
		Rating:          rating,
		Category:        category,
		Images:          images,
		LongDescription: longDesc,
		Url:             url,
		Platform:        platform,
		IsNew:           isNew,
		IsBestseller:    isBestseller,
		ID:              id,
	})
	if err != nil {
		jsonError(w, "Failed to update: "+err.Error(), 500)
		return
	}

	updated, _ := q.GetProduct(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": updated})
}

func (s *Server) handleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, "Invalid ID", 400)
		return
	}
	q := dbgen.New(s.DB)
	q.DeleteProduct(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, "Invalid ID", 400)
		return
	}
	q := dbgen.New(s.DB)
	p, err := q.GetProduct(r.Context(), id)
	if err != nil {
		jsonError(w, "Product not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	products, err := q.ListProducts(r.Context())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	// 10 MB max
	r.ParseMultipartForm(10 << 20)

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "No file uploaded", 400)
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
	if !allowed[ext] {
		jsonError(w, "Only jpg, png, gif, webp allowed", 400)
		return
	}

	// Generate unique filename
	b := make([]byte, 12)
	rand.Read(b)
	filename := hex.EncodeToString(b) + ext

	dst, err := os.Create(filepath.Join(s.UploadsDir, filename))
	if err != nil {
		jsonError(w, "Failed to save file", 500)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		jsonError(w, "Failed to write file", 500)
		return
	}

	url := "/uploads/" + filename
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": url})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": msg})
}
