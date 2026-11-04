// Package audiostream serves one authorized local audio attachment over loopback.
package audiostream

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type target struct{ site, article, name, contentType string }
type Server struct {
	mu       sync.RWMutex
	listener net.Listener
	http     *http.Server
	open     func(string, string, string) (*os.File, error)
	token    string
	current  target
}

func New(open func(string, string, string) (*os.File, error)) (*Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: listener, open: open}
	s.http = &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.http.Serve(listener) }()
	return s, nil
}
func (s *Server) URL(site, article, name string) (string, error) {
	return s.URLWithType(site, article, name, "")
}
func (s *Server) URLWithType(site, article, name, contentType string) (string, error) {
	if contentType != "" && (!strings.HasPrefix(contentType, "audio/") && !strings.HasPrefix(contentType, "video/") || strings.ContainsAny(contentType, "\r\n;")) {
		return "", errors.New("媒体类型无效")
	}
	f, err := s.open(site, article, name)
	if err != nil {
		return "", err
	}
	f.Close()
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", err
	}
	token := hex.EncodeToString(key)
	s.mu.Lock()
	s.token = token
	s.current = target{site, article, name, contentType}
	s.mu.Unlock()
	return "http://" + s.listener.Addr().String() + "/" + token, nil
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Host != s.listener.Addr().String() || (r.Method != "GET" && r.Method != "HEAD") {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	token, item := s.token, s.current
	s.mu.RUnlock()
	if token == "" || r.URL.Path != "/"+token {
		http.NotFound(w, r)
		return
	}
	f, err := s.open(item.site, item.article, item.name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	contentType := item.contentType
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(item.name)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, item.name, info.ModTime(), f)
}
func (s *Server) Close() error {
	if s == nil || s.http == nil {
		return errors.New("server is not running")
	}
	return s.http.Close()
}

// Revoke prevents new requests for the current stream.
func (s *Server) Revoke(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && address == "http://"+s.listener.Addr().String()+"/"+s.token {
		s.token = ""
		return true
	}
	return false
}
