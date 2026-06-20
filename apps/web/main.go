package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"trigger/apps/web/config"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var tmplFS embed.FS

var homeTmpl = parsePage("home.html")

func parsePage(name string) *template.Template {
	return template.Must(template.ParseFS(tmplFS,
		"templates/layout.html",
		"templates/partials.html",
		"templates/"+name,
	))
}

type pageData struct {
	Title  string
	Active string
}

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func main() {
	cfg, err := config.LoadConfig("config")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", handleHome)
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /manifest.json", serveStatic("static/manifest.json", "application/manifest+json"))
	mux.HandleFunc("GET /sw.js", serveStatic("static/sw.js", "text/javascript"))

	addr := ":" + cfg.Port
	log.Printf("listening on http://localhost%s (base URL %s)", addr, cfg.BaseURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, homeTmpl, pageData{Title: "Trigger AI", Active: "home"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:    "ok",
		Service:   "trigger-web",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func serveStatic(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := staticFS.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(b)
	}
}

func renderPage(w http.ResponseWriter, r *http.Request, t *template.Template, data pageData) {
	name := "layout"
	if r.Header.Get("HX-Request") == "true" {
		name = "content"
	}
	renderTemplate(w, t, name, data)
}

func renderTemplate(w http.ResponseWriter, t *template.Template, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
