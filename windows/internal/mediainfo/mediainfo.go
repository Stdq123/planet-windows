// Package mediainfo extracts duration without decoding an entire media file.
package mediainfo

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Duration returns whole seconds when a supported probe can determine duration.
// WAV is handled locally, then Windows properties and an optional ffprobe are tried.
func Duration(path string) *int {
	if value := wavDuration(path); value != nil {
		return value
	}
	if value := windowsDuration(path); value != nil {
		return value
	}
	executable, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", "-i", path)
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil || len(output) > 256 {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 10*365*24*3600 {
		return nil
	}
	seconds := int(value)
	return &seconds
}
func wavDuration(path string) *int {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	var head [12]byte
	if _, err = io.ReadFull(f, head[:]); err != nil || string(head[:4]) != "RIFF" || string(head[8:]) != "WAVE" {
		return nil
	}
	var byteRate uint32
	var dataSize uint32
	var foundData bool
	for offset := int64(12); offset+8 <= info.Size(); {
		var h [8]byte
		if _, err = f.ReadAt(h[:], offset); err != nil {
			return nil
		}
		size := binary.LittleEndian.Uint32(h[4:])
		start := offset + 8
		end := start + int64(size)
		if end > info.Size() || end < start {
			return nil
		}
		if string(h[:4]) == "fmt " && size >= 16 {
			var fmtBytes [16]byte
			if _, err = f.ReadAt(fmtBytes[:], start); err != nil {
				return nil
			}
			byteRate = binary.LittleEndian.Uint32(fmtBytes[8:12])
		}
		if string(h[:4]) == "data" {
			dataSize = size
			foundData = true
		}
		offset = end + int64(size%2)
	}
	if byteRate == 0 || !foundData {
		return nil
	}
	seconds := int(dataSize / byteRate)
	return &seconds
}
