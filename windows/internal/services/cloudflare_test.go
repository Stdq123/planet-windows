package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudflareDirectUploadUsesProjectTokenAndManifest(t *testing.T) {
	const account = "0123456789abcdef0123456789abcdef"
	var manifest map[string]string
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		want := "Bearer secret"
		if strings.HasPrefix(r.URL.Path, "/pages/assets/") {
			want = "Bearer upload-jwt"
		}
		if r.Header.Get("Authorization") != want {
			t.Errorf("wrong authorization for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/accounts/" + account + "/pages/projects/my-site":
			io.WriteString(w, `{"success":true,"result":{"name":"my-site","production_branch":"main"}}`)
		case "/accounts/" + account + "/pages/projects/my-site/upload-token":
			io.WriteString(w, `{"success":true,"result":{"jwt":"upload-jwt"}}`)
		case "/pages/assets/check-missing":
			var body struct {
				Hashes []string `json:"hashes"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": body.Hashes})
		case "/pages/assets/upload":
			var body []struct {
				Key    string `json:"key"`
				Value  string `json:"value"`
				Base64 bool   `json:"base64"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) == 0 || !body[0].Base64 {
				t.Errorf("invalid upload: %v %+v", err, body)
			}
			io.WriteString(w, `{"success":true}`)
		case "/pages/assets/upsert-hashes":
			io.WriteString(w, `{"success":true}`)
		case "/accounts/" + account + "/pages/projects/my-site/deployments":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal([]byte(r.FormValue("manifest")), &manifest); err != nil {
				t.Error(err)
			}
			io.WriteString(w, `{"success":true,"result":{"url":"https://abc.my-site.pages.dev","environment":"production","aliases":["my-site.pages.dev"]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := newCloudflareClient(server.Client(), server.URL).Deploy(dir, account, "my-site", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://my-site.pages.dev/" || manifest["/index.html"] == "" || len(calls) != 6 {
		t.Fatalf("result=%s manifest=%v calls=%v", url, manifest, calls)
	}
}

func TestCloudflareDoesNotCreateProjectOnAuthorizationFailure(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := newCloudflareClient(server.Client(), server.URL).Deploy(dir, "0123456789abcdef0123456789abcdef", "my-site", "secret")
	if err == nil || count != 1 {
		t.Fatalf("expected safe failure, got %v after %d calls", err, count)
	}
}

func TestCloudflareRejectsOversizedAssetsBeforeNetwork(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count++ }))
	defer server.Close()
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "huge.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate((25 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = newCloudflareClient(server.Client(), server.URL).Deploy(dir, "0123456789abcdef0123456789abcdef", "my-site", "secret")
	if err == nil || count != 0 {
		t.Fatalf("oversized asset was sent: %v %d", err, count)
	}
}

func TestCloudflareConfigRoundTripProtectsToken(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Name: "my-site", Server: "0123456789abcdef0123456789abcdef", Token: "private-cloudflare-token", AutoDeploy: true}
	if err := SaveConfig(root, "site-1", "cloudflare", cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "integrations", "cloudflare", "site-1.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), cfg.Token) {
		t.Fatal("token stored in clear text")
	}
	got, err := LoadConfig(root, "site-1", "cloudflare")
	if err != nil || got != cfg {
		t.Fatalf("config mismatch: %+v %v", got, err)
	}
}
