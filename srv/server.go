package srv

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	return srv, nil
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

func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /product/{id}", s.handleProductDetail)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /admin", s.handleAdmin)
	mux.HandleFunc("POST /api/add", s.handleAddProduct)
	mux.HandleFunc("POST /api/delete/{id}", s.handleDeleteProduct)
	mux.HandleFunc("GET /api/products", s.handleListProducts)
	mux.HandleFunc("GET /img", handleImageProxy)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

var funcMap = template.FuncMap{
	"lower": strings.ToLower,
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
	"add": func(a, b int) int { return a + b },
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "..."
	},
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("home.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "home.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{
		"Products":   products,
		"Categories": catOrder,
		"ByCategory": catMap,
	})
}

func (s *Server) handleProductDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", 400)
		return
	}
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": msg})
}
