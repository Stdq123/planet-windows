package localapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"windows/internal/store"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestLocalAPIServerCRUDAndBasicAuth(t *testing.T) {
	root := t.TempDir()
	port := freePort(t)
	if _, err := SaveConfig(root, true, port, true, "Planet", "secret-pass"); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Backend{
		OpenStore: func() (*store.Store, error) { return store.Open(root) },
	})
	if err := srv.Sync(root); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	// Unauthenticated request returns 401
	resp, err := http.Get(baseURL + "/v0/ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without Basic Auth, got %d", resp.StatusCode)
	}

	req := func(method, path string, body any) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		r, err := http.NewRequest(method, baseURL+path, &buf)
		if err != nil {
			t.Fatal(err)
		}
		r.SetBasicAuth("Planet", "secret-pass")
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Create planet
	createRes := req(http.MethodPost, "/v0/planets/my", map[string]string{"name": "API Blog", "about": "From REST"})
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 create planet, got %d", createRes.StatusCode)
	}
	var createdSite store.Site
	if err := json.NewDecoder(createRes.Body).Decode(&createdSite); err != nil || createdSite.Name != "API Blog" {
		t.Fatalf("decode created site: %+v %v", createdSite, err)
	}

	// Create article
	artRes := req(http.MethodPost, "/v0/planets/my/"+createdSite.ID+"/articles", map[string]string{"title": "REST Article", "content": "Hello API World"})
	defer artRes.Body.Close()
	if artRes.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 create article, got %d", artRes.StatusCode)
	}
	var createdArt store.Article
	if err := json.NewDecoder(artRes.Body).Decode(&createdArt); err != nil || createdArt.Title != "REST Article" {
		t.Fatalf("decode created article: %+v %v", createdArt, err)
	}

	// Search
	searchRes := req(http.MethodGet, "/v0/search?q=API", nil)
	defer searchRes.Body.Close()
	if searchRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 search, got %d", searchRes.StatusCode)
	}

	st, err := srv.Status(root)
	if err != nil || !st.Running || len(st.Logs) < 3 {
		t.Fatalf("expected running server with logs, got %+v (%v)", st, err)
	}
}
