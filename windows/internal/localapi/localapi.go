// Package localapi implements Planet's local RESTful HTTP API (/v0/...) and request log console.
package localapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"windows/internal/store"
)

type Config struct {
	Enabled     bool   `json:"enabled"`
	Port        int    `json:"port"`
	AuthEnabled bool   `json:"authEnabled"`
	Username    string `json:"username"`
	HasPasscode bool   `json:"hasPasscode"`
}

type storedConfig struct {
	Enabled     bool   `json:"enabled"`
	Port        int    `json:"port"`
	AuthEnabled bool   `json:"authEnabled"`
	Username    string `json:"username"`
	Passcode    string `json:"passcode,omitempty"`
}

type LogEntry struct {
	Time       string `json:"time"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"durationMs"`
}

type Status struct {
	Running     bool       `json:"running"`
	Enabled     bool       `json:"enabled"`
	Port        int        `json:"port"`
	AuthEnabled bool       `json:"authEnabled"`
	Username    string     `json:"username"`
	HasPasscode bool       `json:"hasPasscode"`
	Logs        []LogEntry `json:"logs"`
}

type Backend struct {
	OpenStore   func() (*store.Store, error)
	PublishSite func(siteID string) error
}

type Server struct {
	mu       sync.Mutex
	backend  Backend
	server   *http.Server
	listener net.Listener
	running  bool
	port     int
	logs     []LogEntry
}

func NewServer(backend Backend) *Server {
	return &Server{backend: backend, logs: []LogEntry{}}
}

func LoadConfig(root string) (Config, error) {
	sc, err := loadStoredConfig(root)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Enabled:     sc.Enabled,
		Port:        sc.Port,
		AuthEnabled: sc.AuthEnabled,
		Username:    sc.Username,
		HasPasscode: sc.Passcode != "",
	}, nil
}

func loadStoredConfig(root string) (storedConfig, error) {
	path := filepath.Join(root, "integrations", "localapi", "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storedConfig{Port: 8086, Username: "Planet"}, nil
	}
	if err != nil {
		return storedConfig{}, err
	}
	var sc storedConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return storedConfig{}, errors.New("本地 API 配置无效")
	}
	if sc.Port <= 0 || sc.Port > 65535 {
		sc.Port = 8086
	}
	if strings.TrimSpace(sc.Username) == "" {
		sc.Username = "Planet"
	}
	return sc, nil
}

func SaveConfig(root string, enabled bool, port int, authEnabled bool, username, passcode string) (Config, error) {
	if port < 1024 || port > 65535 {
		return Config{}, errors.New("API 端口须在 1024 到 65535 之间")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "Planet"
	}
	passcode = strings.TrimSpace(passcode)
	old, _ := loadStoredConfig(root)
	if passcode == "" {
		passcode = old.Passcode
	}
	if authEnabled && passcode == "" {
		return Config{}, errors.New("启用 API 认证时请设置访问口令")
	}
	dir := filepath.Join(root, "integrations", "localapi")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	sc := storedConfig{
		Enabled:     enabled,
		Port:        port,
		AuthEnabled: authEnabled,
		Username:    username,
		Passcode:    passcode,
	}
	raw, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0600); err != nil {
		return Config{}, err
	}
	return Config{
		Enabled:     sc.Enabled,
		Port:        sc.Port,
		AuthEnabled: sc.AuthEnabled,
		Username:    sc.Username,
		HasPasscode: sc.Passcode != "",
	}, nil
}

func (srv *Server) Status(root string) (Status, error) {
	cfg, err := LoadConfig(root)
	if err != nil {
		return Status{}, err
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	logsCopy := append([]LogEntry(nil), srv.logs...)
	port := cfg.Port
	if srv.running && srv.port > 0 {
		port = srv.port
	}
	return Status{
		Running:     srv.running,
		Enabled:     cfg.Enabled,
		Port:        port,
		AuthEnabled: cfg.AuthEnabled,
		Username:    cfg.Username,
		HasPasscode: cfg.HasPasscode,
		Logs:        logsCopy,
	}, nil
}

func (srv *Server) ClearLogs() {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.logs = []LogEntry{}
}

func (srv *Server) appendLog(entry LogEntry) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.logs = append(srv.logs, entry)
	if len(srv.logs) > 200 {
		srv.logs = srv.logs[len(srv.logs)-200:]
	}
}

func (srv *Server) Sync(root string) error {
	sc, err := loadStoredConfig(root)
	if err != nil {
		return err
	}
	srv.mu.Lock()
	if srv.running {
		_ = srv.stopLocked()
	}
	if !sc.Enabled {
		srv.mu.Unlock()
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", sc.Port))
	if err != nil {
		srv.mu.Unlock()
		return fmt.Errorf("无法监听本地 API 端口 %d：%w", sc.Port, err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	httpSrv := &http.Server{
		Handler:           srv.buildHandler(root),
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv.listener = ln
	srv.server = httpSrv
	srv.running = true
	srv.port = actualPort
	srv.mu.Unlock()

	go func() {
		_ = httpSrv.Serve(ln)
	}()
	return nil
}

func (srv *Server) Close() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.stopLocked()
}

func (srv *Server) stopLocked() error {
	if !srv.running {
		return nil
	}
	srv.running = false
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var err error
	if srv.server != nil {
		err = srv.server.Shutdown(ctx)
		srv.server = nil
	}
	if srv.listener != nil {
		_ = srv.listener.Close()
		srv.listener = nil
	}
	return err
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (srv *Server) buildHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			srv.appendLog(LogEntry{
				Time:       start.Format("15:04:05"),
				Method:     r.Method,
				Path:       r.URL.RequestURI(),
				Status:     rec.status,
				DurationMs: time.Since(start).Milliseconds(),
			})
		}()

		sc, err := loadStoredConfig(root)
		if err == nil && sc.AuthEnabled && sc.Passcode != "" {
			user, pass, ok := r.BasicAuth()
			userOK := subtle.ConstantTimeCompare([]byte(user), []byte(sc.Username)) == 1
			passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(sc.Passcode)) == 1
			if !ok || !userOK || !passOK {
				rec.Header().Set("WWW-Authenticate", `Basic realm="Planet API"`)
				writeJSON(rec, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				return
			}
		}

		srv.dispatch(rec, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readFormOrJSON(r *http.Request) map[string]string {
	out := map[string]string{}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(16 << 20); err == nil && r.MultipartForm != nil {
			for k, vals := range r.MultipartForm.Value {
				if len(vals) > 0 {
					out[k] = vals[0]
				}
			}
		}
		return out
	}
	if strings.Contains(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err == nil {
			for k, vals := range r.Form {
				if len(vals) > 0 {
					out[k] = vals[0]
				}
			}
		}
		return out
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err == nil && len(body) > 0 {
		var raw map[string]any
		if json.Unmarshal(body, &raw) == nil {
			for k, v := range raw {
				if s, ok := v.(string); ok {
					out[k] = s
				}
			}
		}
	}
	return out
}

func (srv *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	if path == "/v0/ping" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pong"})
		return
	}
	s, err := srv.backend.OpenStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if path == "/v0/search" && r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		articles, err := s.SearchArticles(q)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sites, _ := s.ListSites()
		var matchedPlanets []store.Site
		lowerQ := strings.ToLower(q)
		if lowerQ != "" {
			for _, site := range sites {
				if strings.Contains(strings.ToLower(site.Name), lowerQ) || strings.Contains(strings.ToLower(site.About), lowerQ) {
					matchedPlanets = append(matchedPlanets, site)
				}
			}
		}
		if matchedPlanets == nil {
			matchedPlanets = []store.Site{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"planets": matchedPlanets, "articles": articles})
		return
	}
	if path == "/v0/planets/my" {
		switch r.Method {
		case http.MethodGet:
			archivedOnly := r.URL.Query().Get("archived") == "true"
			all := r.URL.Query().Get("all") == "true"
			active, err := s.ListSites()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			removed, _ := s.RemovedSites()
			if archivedOnly {
				writeJSON(w, http.StatusOK, removed)
				return
			}
			if all {
				writeJSON(w, http.StatusOK, append(active, removed...))
				return
			}
			writeJSON(w, http.StatusOK, active)
			return
		case http.MethodPost:
			fields := readFormOrJSON(r)
			name := strings.TrimSpace(fields["name"])
			if name == "" {
				name = strings.TrimSpace(fields["title"])
			}
			site, err := s.CreateSite(name, fields["about"])
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, site)
			return
		}
	}
	const prefix = "/v0/planets/my/"
	if !strings.HasPrefix(path, prefix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not Found"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	siteID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			site, ok := findSite(s, siteID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
				return
			}
			writeJSON(w, http.StatusOK, site)
			return
		case http.MethodPost:
			fields := readFormOrJSON(r)
			current, ok := findSite(s, siteID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
				return
			}
			name := strings.TrimSpace(fields["name"])
			if name == "" {
				name = current.Name
			}
			about, hasAbout := fields["about"]
			if !hasAbout {
				about = current.About
			}
			updated, err := s.UpdateSite(current.ID, name, about)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, updated)
			return
		case http.MethodDelete:
			current, ok := findSite(s, siteID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
				return
			}
			if err := s.RemoveSite(current.ID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, current)
			return
		}
	}
	if len(parts) == 2 && parts[1] == "publish" && r.Method == http.MethodPost {
		current, ok := findSite(s, siteID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
			return
		}
		if srv.backend.PublishSite != nil {
			if err := srv.backend.PublishSite(current.ID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		updated, _ := findSite(s, current.ID)
		writeJSON(w, http.StatusOK, updated)
		return
	}
	if len(parts) == 2 && parts[1] == "articles" {
		current, ok := findSite(s, siteID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			articles, err := s.ListArticles(current.ID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, articles)
			return
		case http.MethodPost:
			fields := readFormOrJSON(r)
			title := strings.TrimSpace(fields["title"])
			content := fields["content"]
			if title == "" && strings.TrimSpace(content) != "" {
				title = "Untitled"
			}
			article, err := s.SaveArticle(current.ID, "", title, content)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, article)
			return
		}
	}
	if len(parts) == 3 && parts[1] == "articles" {
		current, ok := findSite(s, siteID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
			return
		}
		article, ok := findArticle(s, current.ID, parts[2])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Article not found"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, article)
			return
		case http.MethodPost:
			fields := readFormOrJSON(r)
			title, hasTitle := fields["title"]
			if !hasTitle || strings.TrimSpace(title) == "" {
				title = article.Title
			}
			content, hasContent := fields["content"]
			if !hasContent {
				content = article.Content
			}
			updated, err := s.SaveArticle(current.ID, article.ID, title, content)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, updated)
			return
		case http.MethodDelete:
			if err := s.DeleteArticle(current.ID, article.ID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, article)
			return
		}
	}
	if len(parts) == 4 && parts[1] == "articles" && parts[3] == "attachments" && r.Method == http.MethodGet {
		current, ok := findSite(s, siteID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Planet not found"})
			return
		}
		article, ok := findArticle(s, current.ID, parts[2])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Article not found"})
			return
		}
		att := article.Attachments
		if att == nil {
			att = []string{}
		}
		writeJSON(w, http.StatusOK, att)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not Found"})
}

func findSite(s *store.Store, selector string) (store.Site, bool) {
	sites, err := s.ListSites()
	if err != nil {
		return store.Site{}, false
	}
	removed, _ := s.RemovedSites()
	all := append(sites, removed...)
	for _, site := range all {
		if strings.EqualFold(site.ID, selector) || (site.Slug != "" && strings.EqualFold(site.Slug, selector)) || strings.EqualFold(site.Name, selector) {
			return site, true
		}
	}
	for _, site := range all {
		if len(selector) >= 4 && strings.HasPrefix(strings.ToLower(site.ID), strings.ToLower(selector)) {
			return site, true
		}
	}
	return store.Site{}, false
}

func findArticle(s *store.Store, siteID, selector string) (store.Article, bool) {
	articles, err := s.ListArticles(siteID)
	if err != nil {
		return store.Article{}, false
	}
	for _, a := range articles {
		if strings.EqualFold(a.ID, selector) || (a.ArticleReference != "" && strings.EqualFold(a.ArticleReference, selector)) || strings.EqualFold(a.Title, selector) {
			return a, true
		}
	}
	for _, a := range articles {
		if len(selector) >= 4 && strings.HasPrefix(strings.ToLower(a.ID), strings.ToLower(selector)) {
			return a, true
		}
	}
	return store.Article{}, false
}
