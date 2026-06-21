// Package api owns the HTTP transport: the Server struct, routing, handlers,
// and template rendering. Domain usecases are injected here; handlers do only
// auth/validation, call a usecase, and render the response.
package api

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"trigger/apps/web/app/sales/adapter"
	"trigger/apps/web/app/sales/usecase"
)

// Server holds injected dependencies and parsed templates. All handlers are
// methods on Server.
type Server struct {
	sales  *usecase.Usecase
	pages  map[string]*template.Template
	static embed.FS
}

// NewServer parses the page templates out of templateFS and returns a Server
// wired to the injected sales usecase.
func NewServer(salesUC *usecase.Usecase, templateFS, staticFS embed.FS) *Server {
	return &Server{
		sales: salesUC,
		pages: map[string]*template.Template{
			"home":       parsePage(templateFS, "home.html"),
			"activities": parsePage(templateFS, "activities.html"),
		},
		static: staticFS,
	}
}

func parsePage(templateFS embed.FS, name string) *template.Template {
	return template.Must(template.ParseFS(templateFS,
		"templates/layout.html",
		"templates/partials.html",
		"templates/"+name,
	))
}

// pageData is the common payload every full-page render needs (layout title +
// active nav link).
type pageData struct {
	Title  string
	Active string
}

// RegisterRoutes wires every route onto the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /activities", s.handleActivities)
	mux.Handle("GET /static/", http.FileServerFS(s.static))
	mux.HandleFunc("GET /manifest.json", s.serveStatic("static/manifest.json", "application/manifest+json"))
	mux.HandleFunc("GET /sw.js", s.serveStatic("static/sw.js", "text/javascript"))
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "home", pageData{Title: "Trigger AI", Active: "home"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "ok",
		"service":   "trigger-web",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleActivities renders the follow-up activity list. Query params:
//
//	state       overdue|today|planned
//	type        Call|Email|Meeting|To-Do
//	sort        date_deadline|id
//	dir         asc|desc
//	page_size   integer
//	scroll_token opaque keyset cursor from previous response
//
// When scroll_token is set on an HTMX request, returns only the "activity-rows"
// fragment so HTMX can append it into the existing list (infinite scroll).
func (s *Server) handleActivities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := adapter.ActivityListReq{
		ScrollToken: q.Get("scroll_token"),
		Sort:        q.Get("sort"),
		Dir:         q.Get("dir"),
		State:       q.Get("state"),
		Type:        q.Get("type"),
		UserID:      q.Get("user_id"),
		LeadID:      q.Get("lead_id"),
	}
	req.PageSize = 10
	if ps := q.Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			req.PageSize = n
		}
	}

	// loaded and total are carried through the scroll sentinel URL so the
	// counter can update without a full-page swap.
	prevLoaded, _ := strconv.Atoi(q.Get("loaded"))
	prevTotal, _ := strconv.Atoi(q.Get("total"))

	result, err := s.sales.ActivityList(r.Context(), req)
	if err != nil {
		http.Error(w, "load activities: "+err.Error(), http.StatusInternalServerError)
		return
	}

	total := prevTotal
	if result.Count != nil {
		total = *result.Count
	}

	data := activitiesPageData{
		pageData:    pageData{Title: "Activities", Active: "activities"},
		State:       req.State,
		Type:        req.Type,
		Sort:        req.Sort,
		Dir:         req.Dir,
		Result:      result,
		LoadedSoFar: prevLoaded + len(result.Items),
		Total:       total,
	}

	// Infinite scroll: HTMX requests with a scroll token get only the rows
	// fragment; HTMX appends it into the existing list via hx-swap="outerHTML"
	// on the scroll sentinel.
	if req.ScrollToken != "" && r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		t := s.pages["activities"]
		if err := t.ExecuteTemplate(w, "activity-rows", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	s.renderPage(w, r, "activities", data)
}

type activitiesPageData struct {
	pageData
	State       string
	Type        string
	Sort        string
	Dir         string
	Result      adapter.ActivityListResult
	LoadedSoFar int
	Total       int
}

// renderPage executes the named page template. For HTMX requests it swaps only
// the "content" block; for full page loads it renders the whole layout.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, page string, data any) {
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "unknown page: "+page, http.StatusInternalServerError)
		return
	}
	name := "layout"
	if r.Header.Get("HX-Request") == "true" {
		name = "content"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) serveStatic(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := s.static.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(b)
	}
}
