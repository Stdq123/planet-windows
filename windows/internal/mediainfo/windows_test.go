//go:build windows

package mediainfo

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsReadsMP3DurationWithoutExternalProbe(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required to generate a test MP3")
	}
	file := filepath.Join(t.TempDir(), "tone.mp3")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=4", "-c:a", "libmp3lame", "-y", file)
	hideWindow(cmd)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create MP3: %s %v", b, err)
	}
	duration := windowsDuration(file)
	if duration == nil || *duration < 3 || *duration > 5 {
		t.Fatalf("Windows MP3 duration=%v", duration)
	}
}

func TestWindowsExtractsVideoThumbnail(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required to generate a test video")
	}
	file := filepath.Join(t.TempDir(), "sample.mp4")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:d=2", "-c:v", "mpeg4", "-y", file)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create video: %s %v", out, err)
	}
	data, err := VideoThumbnail(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("not a PNG: %x", data)
	}
}
