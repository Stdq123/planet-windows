package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func signed(t *testing.T, key ed25519.PrivateKey, r Release) []byte {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return envelopeBytes(b, ed25519.Sign(key, b))
}

func TestSignedManifestRejectsCaseVariantDuplicate(t *testing.T) {
	cfg, r, client, manifest, _, key := fixture(t)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"VERSION":"0.5.0"}`)...)
	*manifest = envelopeBytes(raw, ed25519.Sign(key, raw))
	if got, err := client.Check(context.Background(), cfg, "0.3.0", time.Now()); err == nil {
		t.Fatalf("accepted a signed manifest with ambiguous version fields: %q", got.Version)
	}
}

func TestVerifiedDownloadActualInstaller(t *testing.T) {
	installer := os.Getenv("PLANET_UPDATE_TEST_INSTALLER")
	if installer == "" {
		t.Skip("set PLANET_UPDATE_TEST_INSTALLER to run the local-only installer download test")
	}
	f, err := os.Open(installer)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	size, err := io.Copy(h, io.LimitReader(f, MaxInstallerSize+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || size == 0 || size > MaxInstallerSize {
		t.Fatalf("cannot hash installer fixture: size=%d read=%v close=%v", size, err, closeErr)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var manifest []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/feed.json" {
			_, _ = w.Write(manifest)
			return
		}
		if q.URL.Path != "/setup.exe" {
			http.NotFound(w, q)
			return
		}
		source, e := os.Open(installer)
		if e != nil {
			http.Error(w, "test fixture unavailable", http.StatusInternalServerError)
			return
		}
		defer source.Close()
		_, _ = io.Copy(w, source)
	}))
	defer server.Close()
	r := Release{Schema: 1, App: AppID, Platform: "windows-amd64", Version: "0.4.0", MinimumVersion: "0.2.0", URL: server.URL + "/setup.exe", SHA256: hex.EncodeToString(h.Sum(nil)), Size: size, PublishedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	manifest = signed(t, key, r)
	cfg := Config{FeedURL: server.URL + "/feed.json", PublicKey: base64.StdEncoding.EncodeToString(pub)}
	path, err := newClient(server.Client()).Download(context.Background(), cfg, "0.3.0", r, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyDownloaded(path, r); err != nil {
		t.Fatalf("downloaded installer does not match signed fixture: %v", err)
	}
}

func TestDownloadDoesNotPublishConcurrentTempMutation(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("VALID-SIGNED-DOWNLOAD-BYTES")
	want := sha256.Sum256(body)
	firstChunk, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	release := func() { resumeOnce.Do(func() { close(resume) }) }
	var manifest []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/feed.json" {
			_, _ = w.Write(manifest)
			return
		}
		_, _ = w.Write(body[:10])
		w.(http.Flusher).Flush()
		close(firstChunk)
		<-resume
		_, _ = w.Write(body[10:])
	}))
	defer func() { release(); server.Close() }()
	r := Release{Schema: 1, App: AppID, Platform: "windows-amd64", Version: "0.4.0", MinimumVersion: "0.2.0", URL: server.URL + "/setup.exe", SHA256: hex.EncodeToString(want[:]), Size: int64(len(body)), PublishedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	manifest = signed(t, key, r)
	cfg := Config{FeedURL: server.URL + "/feed.json", PublicKey: base64.StdEncoding.EncodeToString(pub)}
	dir := t.TempDir()
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		path, err := newClient(server.Client()).Download(ctx, cfg, "0.3.0", r, dir, time.Now())
		done <- result{path, err}
	}()
	select {
	case <-firstChunk:
	case <-ctx.Done():
		t.Fatal("download did not reach the controlled checkpoint")
	}
	var target string
	for i := 0; i < 500; i++ {
		paths, err := filepath.Glob(filepath.Join(dir, ".planet-update-*.part"))
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) > 0 {
			if info, err := os.Stat(paths[0]); err == nil && info.Size() >= 10 {
				target = paths[0]
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if target == "" {
		t.Fatal("temporary download did not appear")
	}
	// Simulate another process modifying the partially downloaded file. A
	// sharing violation is also acceptable: it prevents the mutation itself.
	if f, err := os.OpenFile(target, os.O_WRONLY, 0); err == nil {
		_, writeErr := f.WriteAt([]byte("X"), 0)
		_ = f.Close()
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	release()
	got := <-done
	if got.err != nil {
		files, _ := filepath.Glob(filepath.Join(dir, "*.exe"))
		if len(files) != 0 {
			t.Fatal("failed download published an executable")
		}
		return
	}
	final, err := os.ReadFile(got.path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := sha256.Sum256(final); actual != want {
		t.Fatalf("published file hash %x differs from signed hash %x", actual, want)
	}
}
func envelopeBytes(b, sig []byte) []byte {
	v, _ := json.Marshal(map[string][]byte{"payload": b, "signature": sig})
	return v
}
func fixture(t *testing.T) (Config, Release, *Client, *[]byte, *[]byte, ed25519.PrivateKey) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	body := []byte("test installer bytes")
	sum := sha256.Sum256(body)
	var manifest []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/feed.json" {
			w.Write(manifest)
		} else if q.URL.Path == "/setup.exe" {
			w.Write(body)
		} else {
			http.NotFound(w, q)
		}
	}))
	t.Cleanup(server.Close)
	r := Release{Schema: 1, App: AppID, Platform: "windows-amd64", Version: "0.4.0", MinimumVersion: "0.2.0", URL: server.URL + "/setup.exe", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)), PublishedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	manifest = signed(t, key, r)
	cfg := Config{FeedURL: server.URL + "/feed.json", PublicKey: base64.StdEncoding.EncodeToString(pub)}
	return cfg, r, newClient(server.Client()), &manifest, &body, key
}
func TestSignedCheckAndVerifiedDownload(t *testing.T) {
	cfg, r, c, _, _, _ := fixture(t)
	got, e := c.Check(context.Background(), cfg, "0.3.0", time.Now())
	if e != nil || got.SHA256 != r.SHA256 {
		t.Fatal(got, e)
	}
	path, e := c.Download(context.Background(), cfg, "0.3.0", r, t.TempDir(), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	sum := sha256.Sum256(b)
	if e != nil || hex.EncodeToString(sum[:]) != r.SHA256 {
		t.Fatal("download differs", e)
	}
}

func TestPrereleaseUpdatePolicyAndReplayProtection(t *testing.T) {
	for _, tc := range []struct {
		name, current, target, minimum, highest string
		allowed                                 bool
	}{
		{"alpha upgrade", "0.0.1-alpha.1", "0.0.1-alpha.2", "0.0.1-alpha.1", "", true},
		{"numeric identifier ordering", "0.0.1-alpha.2", "0.0.1-alpha.10", "0.0.1-alpha.1", "0.0.1-alpha.2", true},
		{"stable promotion", "0.0.1-alpha.2", "0.0.1", "0.0.1-alpha.1", "0.0.1-alpha.2", true},
		{"stable must not opt into alpha", "0.0.1", "0.0.2-alpha.1", "0.0.1", "", false},
		{"alpha downgrade", "0.0.1-alpha.10", "0.0.1-alpha.2", "0.0.1-alpha.1", "", false},
		{"replayed alpha", "0.0.1-alpha.1", "0.0.1-alpha.2", "0.0.1-alpha.1", "0.0.1-alpha.10", false},
		{"stable minimum excludes alpha", "0.0.1-alpha.1", "0.0.1-alpha.2", "0.0.1", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, r, client, manifest, _, key := fixture(t)
			r.Version, r.MinimumVersion = tc.target, tc.minimum
			cfg.HighestVersion, cfg.HighestSHA256 = tc.highest, r.SHA256
			*manifest = signed(t, key, r)
			got, err := client.Check(context.Background(), cfg, tc.current, time.Now())
			if (err == nil) != tc.allowed || (err == nil && got.Version != tc.target) {
				t.Fatalf("Check() = %q, %v; allowed=%v", got.Version, err, tc.allowed)
			}
		})
	}
}

func TestSignedPrereleaseDownload(t *testing.T) {
	cfg, r, client, manifest, body, key := fixture(t)
	r.Version, r.MinimumVersion = "0.0.1-alpha.2", "0.0.1-alpha.1"
	*manifest = signed(t, key, r)
	path, err := client.Download(context.Background(), cfg, "0.0.1-alpha.1", r, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(*body) || !strings.Contains(filepath.Base(path), "0.0.1-alpha.2") {
		t.Fatalf("prerelease installer bytes/name: %q, %v", path, err)
	}
}
func TestManifestRejectsTamperingExpiryReplayAndForeignTargets(t *testing.T) {
	for _, kind := range []string{"signature", "expired", "future", "long-validity", "app", "platform", "downgrade", "replay", "same-version-other-hash", "origin", "min-version", "huge", "duplicate", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			cfg, r, c, manifest, _, key := fixture(t)
			switch kind {
			case "signature":
				*manifest = envelopeBytes([]byte(`{}`), make([]byte, 64))
			case "expired":
				r.ExpiresAt = time.Now().Add(-time.Minute)
			case "future":
				r.PublishedAt = time.Now().Add(time.Hour)
			case "long-validity":
				r.ExpiresAt = time.Now().Add(60 * 24 * time.Hour)
			case "app":
				r.App = "another.app"
			case "platform":
				r.Platform = "linux-amd64"
			case "downgrade":
				r.Version = "0.2.0"
			case "replay":
				cfg.HighestVersion = "0.5.0"
			case "same-version-other-hash":
				cfg.HighestVersion = r.Version
				cfg.HighestSHA256 = strings.Repeat("1", 64)
			case "origin":
				r.URL = "https://other.example/setup.exe"
			case "min-version":
				r.MinimumVersion = "0.9.0"
			case "huge":
				r.Size = MaxInstallerSize + 1
			case "duplicate":
				b, _ := json.Marshal(r)
				b = append([]byte(`{"version":"0.1.0",`), b[1:]...)
				*manifest = envelopeBytes(b, ed25519.Sign(key, b))
			case "trailing":
				*manifest = append(signed(t, key, r), []byte(" {}")...)
			}
			if kind != "signature" && kind != "duplicate" && kind != "trailing" {
				*manifest = signed(t, key, r)
			}
			if _, e := c.Check(context.Background(), cfg, "0.3.0", time.Now()); e == nil {
				t.Fatal("invalid signed update accepted")
			}
		})
	}
}
func TestDownloadRejectsIncompleteChangedOrStaleRelease(t *testing.T) {
	for _, kind := range []string{"truncated", "long", "hash", "stale"} {
		t.Run(kind, func(t *testing.T) {
			cfg, r, c, manifest, body, key := fixture(t)
			switch kind {
			case "truncated":
				*body = []byte("short")
			case "long":
				*body = append(*body, 1)
			case "hash":
				(*body)[0] = 'X'
			case "stale":
				next := r
				next.Version = "0.5.0"
				*manifest = signed(t, key, next)
			}
			dir := t.TempDir()
			if _, e := c.Download(context.Background(), cfg, "0.3.0", r, dir, time.Now()); e == nil {
				t.Fatal("invalid download succeeded")
			}
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".exe") {
					t.Fatal("unverified executable published")
				}
			}
		})
	}
}
func TestRedirectIsNeverFollowed(t *testing.T) {
	cfg, _, c, _, _, _ := fixture(t)
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		requests++
		http.Redirect(w, q, cfg.FeedURL, http.StatusFound)
	}))
	defer server.Close()
	cfg.FeedURL = server.URL + "/feed.json"
	c = newClient(server.Client())
	if _, e := c.Check(context.Background(), cfg, "0.3.0", time.Now()); e == nil || requests != 1 {
		t.Fatal("redirect accepted", e, requests)
	}
}
