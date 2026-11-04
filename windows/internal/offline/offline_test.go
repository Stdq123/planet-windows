package offline

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"windows/internal/feeds"
)

func TestPinPreservesImagesAudioAndVideoWithoutNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/photo.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG\r\n\x1a\nimage"))
		case "/sound.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("audio"))
		case "/clip.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	root := t.TempDir()
	feed := feeds.Feed{Items: []feeds.Item{{ID: "post", Link: server.URL + "/post", Content: `<img src="/photo.png"><video src="/clip.mp4"></video>`, AudioURL: server.URL + "/sound.mp3"}}}
	pinned, snapshot, err := Pin(root, server.URL, feed)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Close()
	if snapshot == "" || len(pinned.Items[0].OfflineMedia) != 3 {
		t.Fatalf("snapshot=%s item=%+v", snapshot, pinned.Items[0])
	}
	html, err := Render(root, pinned.Items[0])
	if err != nil || !strings.Contains(html, "data:image/png;base64,") {
		t.Fatalf("html=%s err=%v", html, err)
	}
	f, err := Open(root, pinned.Items[0].OfflineMedia[server.URL+"/sound.mp3"])
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestPinRejectsFailedMediaAndDoesNotLeaveSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "missing", http.StatusNotFound) }))
	defer server.Close()
	root := t.TempDir()
	feed := feeds.Feed{Items: []feeds.Item{{ID: "one", Content: `<img src="/missing.png">`}}}
	if _, _, err := Pin(root, server.URL, feed); err == nil {
		t.Fatal("missing media accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "offline-media"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("incomplete snapshots=%v err=%v", entries, err)
	}
}
func TestMediaURLCannotTargetDifferentPrivateHost(t *testing.T) {
	if err := allowedURL("https://example.com/feed", "http://127.0.0.1:1234/secret"); err == nil {
		t.Fatal("private network target accepted")
	}
}

func TestPinAndRenderResponsiveImageSourceSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG\r\n\x1a\nimage"))
	}))
	defer server.Close()
	root := t.TempDir()
	feed := feeds.Feed{Items: []feeds.Item{{ID: "one", Content: `<picture><source srcset="/large.png 2x"><img srcset="/small.png 1x"></picture>`}}}
	pinned, _, err := Pin(root, server.URL, feed)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinned.Items[0].OfflineMedia) != 2 {
		t.Fatalf("assets=%+v", pinned.Items[0].OfflineMedia)
	}
	html, err := Render(root, pinned.Items[0])
	if err != nil || strings.Count(html, "data:image/png;base64,") != 2 || strings.Contains(html, "/large.png") {
		t.Fatalf("html=%s err=%v", html, err)
	}
}

func TestRefreshReusesUnchangedMediaAndReplacesChangedContent(t *testing.T) {
	value := "old"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("ETag", `"`+value+`"`)
		if r.Header.Get("If-None-Match") == `"`+value+`"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(value))
	}))
	defer server.Close()
	root := t.TempDir()
	feed := feeds.Feed{Items: []feeds.Item{{ID: "one", AudioURL: server.URL + "/audio.mp3"}}}
	first, _, err := Pin(root, server.URL, feed)
	if err != nil {
		t.Fatal(err)
	}
	old := first.Items[0].OfflineMedia[server.URL+"/audio.mp3"]
	second, _, err := PinWithPrevious(root, server.URL, feed, first)
	if err != nil {
		t.Fatal(err)
	}
	if second.Items[0].OfflineMedia[server.URL+"/audio.mp3"].Snapshot != old.Snapshot || requests != 2 {
		t.Fatalf("unchanged media not reused: %+v", second)
	}
	value = "new"
	third, _, err := PinWithPrevious(root, server.URL, feed, second)
	if err != nil {
		t.Fatal(err)
	}
	if third.Items[0].OfflineMedia[server.URL+"/audio.mp3"].Snapshot == old.Snapshot {
		t.Fatal("changed media reused")
	}
	f, err := Open(root, old)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "old" {
		t.Fatalf("old snapshot changed: %q", data)
	}
}

func TestPinUsesDeclaredTypeForGenericAttachmentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()
	feed := feeds.Feed{Items: []feeds.Item{{ID: "one", Media: []feeds.RemoteMedia{{URL: server.URL + "/download", Type: "audio/mpeg"}}}}}
	result, _, err := Pin(t.TempDir(), server.URL, feed)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].OfflineMedia[server.URL+"/download"].ContentType != "audio/mpeg" {
		t.Fatal("missing declared content type")
	}
}

func TestCollectReclaimsOnlyUnreferencedManagedFiles(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "offline-media")
	dir := filepath.Join(parent, "snapshot-123")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	kept := strings.Repeat("a", 64) + ".media"
	stale := strings.Repeat("b", 64) + ".media"
	for name, data := range map[string]string{kept: "keep", stale: "obsolete", "notes.txt": "outside"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	refs := map[string]map[string]bool{"snapshot-123": {kept: true}}
	usage, err := Collect(root, refs, false)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalBytes != 12 || usage.ReclaimableBytes != 8 {
		t.Fatalf("usage=%+v", usage)
	}
	usage, err = Collect(root, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ReclaimedBytes != 8 {
		t.Fatalf("usage=%+v", usage)
	}
	if _, err = os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
		t.Fatal("stale asset remains")
	}
	for _, name := range []string{kept, "notes.txt"} {
		if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("lost %s: %v", name, err)
		}
	}
}

func TestCollectLeavesUnmanagedAndSymlinkEntriesUntouched(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	parent := filepath.Join(root, "offline-media")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	file := strings.Repeat("a", 64) + ".media"
	outside := filepath.Join(other, file)
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(parent, "snapshot-999")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	u, err := Collect(root, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if u.ReclaimedFiles != 0 {
		t.Fatalf("cleaned symlink target: %+v", u)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside affected: %q %v", data, err)
	}
}
