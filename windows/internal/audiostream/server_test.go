package audiostream

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamAllowsRangesAndRevokesPreviousAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := New(func(site, article, name string) (*os.File, error) {
		if site != "s" || article != "p" || name != "audio.mp3" {
			return nil, os.ErrNotExist
		}
		return os.Open(path)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	first, err := server.URL("s", "p", "audio.mp3")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", first, nil)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || string(b) != "2345" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, b)
	}
	_, err = server.URL("s", "p", "audio.mp3")
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(first)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("old URL status=%d", resp.StatusCode)
	}
}

func TestRevokeOldURLDoesNotInvalidateNewPlayback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.mp3")
	os.WriteFile(path, []byte("abc"), 0600)
	s, err := New(func(string, string, string) (*os.File, error) { return os.Open(path) })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old, _ := s.URL("s", "p", "audio.mp3")
	current, _ := s.URL("s", "p", "audio.mp3")
	s.Revoke(old)
	resp, err := http.Get(current)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("new stream revoked: %d", resp.StatusCode)
	}
	s.Revoke(current)
	resp, err = http.Get(current)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("current stream not revoked: %d", resp.StatusCode)
	}
}

func TestStreamUsesExplicitMediaTypeWithoutFilenameExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cached")
	os.WriteFile(path, []byte("media"), 0600)
	s, err := New(func(string, string, string) (*os.File, error) { return os.Open(path) })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	url, err := s.URLWithType("s", "p", "no-extension", "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("type=%q", resp.Header.Get("Content-Type"))
	}
}
