package mediainfo

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWAVDurationAndInvalidInput(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+16))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint32(8))
	binary.Write(&b, binary.LittleEndian, uint32(8))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(8))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	b.Write(make([]byte, 16))
	path := filepath.Join(t.TempDir(), "tone.wav")
	os.WriteFile(path, b.Bytes(), 0600)
	duration := Duration(path)
	if duration == nil || *duration != 2 {
		t.Fatalf("duration=%v", duration)
	}
	bad := filepath.Join(t.TempDir(), "bad.wav")
	os.WriteFile(bad, []byte("bad"), 0600)
	if d := Duration(bad); d != nil {
		t.Fatalf("invalid duration=%d", *d)
	}
}
