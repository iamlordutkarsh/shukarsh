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
	mux.HandleFunc("GET /admin", s.handleAdmin)
	mux.HandleFunc("POST /api/add", s.handleAddProduct)
	mux.HandleFunc("POST /api/delete/{id}", s.handleDeleteProduct)
	mux.HandleFunc("GET /api/products", s.handleListProducts)
	mux.HandleFunc("GET /img", handleImageProxy)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

var funcMap = template.FuncMap{"lower": strings.ToLower}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	q := dbgen.New(s.DB)
	products, err := q.ListProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.New("home.html").Funcs(funcMap).ParseFiles(filepath.Join(s.TemplatesDir, "home.html"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]any{"Products": products})
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
	mode := r.FormValue("mode") // "url" or "manual"

	var params dbgen.InsertProductParams

	if mode == "manual" {
		// Manual entry
		params = dbgen.InsertProductParams{
			Url:           r.FormValue("url"),
			Platform:      r.FormValue("platform"),
			Title:         r.FormValue("title"),
			Price:         r.FormValue("price"),
			OriginalPrice: r.FormValue("original_price"),
			ImageUrl:      r.FormValue("image_url"),
			Description:   r.FormValue("description"),
			Rating:        r.FormValue("rating"),
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
		// URL scrape mode
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
