//go:build windows

package attachments

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"windows/internal/store"
)

func TestGenerateThumbnailFromRegisteredVideo(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required to generate a test video")
	}
	source := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:d=2", "-c:v", "mpeg4", "-y", source)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("video: %s %v", b, err)
	}
	root := t.TempDir()
	lib, _ := store.Open(root)
	site, _ := lib.CreateSite("Site", "")
	post, _ := lib.SaveArticle(site.ID, "", "Post", "")
	post, err = Add(root, lib, site.ID, post.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	os.Mkdir(filepath.Join(dest, post.ID), 0700)
	if err = GenerateVideoThumbnail(root, site, post, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, post.ID, "_videoThumbnail.png"))
	if err != nil || len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("thumbnail: %x %v", data, err)
	}
}
