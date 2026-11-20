package siteimage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodeTestPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func makeICO(entries [][]byte, dims [][2]int, bits []int) []byte {
	var buf bytes.Buffer
	header := make([]byte, 6+len(entries)*16)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], uint16(len(entries)))
	offset := uint32(len(header))
	for i, payload := range entries {
		off := 6 + i*16
		w, h := dims[i][0], dims[i][1]
		if w == 256 {
			w = 0
		}
		if h == 256 {
			h = 0
		}
		header[off] = byte(w)
		header[off+1] = byte(h)
		binary.LittleEndian.PutUint16(header[off+4:off+6], 1)
		binary.LittleEndian.PutUint16(header[off+6:off+8], uint16(bits[i]))
		binary.LittleEndian.PutUint32(header[off+8:off+12], uint32(len(payload)))
		binary.LittleEndian.PutUint32(header[off+12:off+16], offset)
		offset += uint32(len(payload))
	}
	buf.Write(header)
	for _, payload := range entries {
		buf.Write(payload)
	}
	return buf.Bytes()
}

func TestNormalizeIconDownscalesOversizedImagesAndPreservesSmallOnes(t *testing.T) {
	small := encodeTestPNG(t, 48, 48, color.NRGBA{R: 12, G: 120, B: 240, A: 255})
	encoded, err := NormalizeIcon(small)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := DecodePNG(encoded)
	if err != nil || img.Bounds().Dx() != 48 || img.Bounds().Dy() != 48 {
		t.Fatalf("small icon altered: bounds=%v err=%v", img.Bounds(), err)
	}

	var largeJPEG bytes.Buffer
	large := image.NewRGBA(image.Rect(0, 0, 512, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 512; x++ {
			large.SetRGBA(x, y, color.RGBA{R: 108, G: 166, B: 212, A: 255})
		}
	}
	if err := jpeg.Encode(&largeJPEG, large, nil); err != nil {
		t.Fatal(err)
	}
	encoded, err = NormalizeIcon(largeJPEG.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, _, err = DecodePNG(encoded)
	if err != nil || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 64 {
		t.Fatalf("large icon size=%v err=%v", img.Bounds(), err)
	}
	if _, _, _, a := img.At(64, 32).RGBA(); a != 0xffff {
		t.Fatalf("downscaled opaque JPEG lost alpha: a=%d", a)
	}

	// Regression: 144x144 opaque PNG downscaled to 128x128 must keep A=255 (no uint8 overflow).
	avatar144 := encodeTestPNG(t, 144, 144, color.NRGBA{R: 108, G: 166, B: 212, A: 255})
	encoded, err = NormalizeIcon(avatar144)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err = DecodePNG(encoded)
	if err != nil || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("144x144 downscale size=%v err=%v", img.Bounds(), err)
	}
	r, g, b, a := img.At(64, 64).RGBA()
	if r>>8 != 108 || g>>8 != 166 || b>>8 != 212 || a>>8 != 255 {
		t.Fatalf("downscaled pixel corrupted: rgba=%d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestNormalizeIconDecodesMultiEntryPNGICO(t *testing.T) {
	small := encodeTestPNG(t, 16, 16, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	larger := encodeTestPNG(t, 64, 64, color.NRGBA{R: 0, G: 180, B: 60, A: 255})
	ico := makeICO([][]byte{small, larger}, [][2]int{{16, 16}, {64, 64}}, []int{32, 32})

	encoded, err := NormalizeIcon(ico)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := DecodePNG(encoded)
	if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("expected 64x64 entry, got bounds=%v err=%v", img.Bounds(), err)
	}
	r, g, b, _ := img.At(10, 10).RGBA()
	if r>>8 != 0 || g>>8 != 180 || b>>8 != 60 {
		t.Fatalf("unexpected pixel color: %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestNormalizeIconDecodesBMPICO24BitWithANDMask(t *testing.T) {
	// 2x2 24-bit bottom-up BMP DIB with top-left pixel masked transparent.
	var dib bytes.Buffer
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:4], 40)
	binary.LittleEndian.PutUint32(hdr[4:8], 2)
	binary.LittleEndian.PutUint32(hdr[8:12], 4) // 2 * height
	binary.LittleEndian.PutUint16(hdr[12:14], 1)
	binary.LittleEndian.PutUint16(hdr[14:16], 24)
	dib.Write(hdr)
	// Row 0 (bottom, y=1): two pixels BGR + 2 padding bytes = 8 bytes
	dib.Write([]byte{10, 20, 30, 40, 50, 60, 0, 0})
	// Row 1 (top, y=0): two pixels BGR + 2 padding bytes = 8 bytes
	dib.Write([]byte{70, 80, 90, 100, 110, 120, 0, 0})
	// AND mask (4 bytes per row): bottom row opaque (0x00), top row left pixel transparent (0x80)
	dib.Write([]byte{0x00, 0x00, 0x00, 0x00})
	dib.Write([]byte{0x80, 0x00, 0x00, 0x00})

	ico := makeICO([][]byte{dib.Bytes()}, [][2]int{{2, 2}}, []int{24})
	encoded, err := NormalizeIcon(ico)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := DecodePNG(encoded)
	if err != nil || img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("bounds=%v err=%v", img.Bounds(), err)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatalf("expected transparent top-left pixel, alpha=%d", a)
	}
	r, g, b, a := img.At(1, 0).RGBA()
	if r>>8 != 120 || g>>8 != 110 || b>>8 != 100 || a>>8 != 255 {
		t.Fatalf("unexpected top-right pixel: %d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestNormalizeIconDecodesBMPICO32BitAndPaletted(t *testing.T) {
	// 1x1 32-bit BGRA
	var dib32 bytes.Buffer
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:4], 40)
	binary.LittleEndian.PutUint32(hdr[4:8], 1)
	binary.LittleEndian.PutUint32(hdr[8:12], 2)
	binary.LittleEndian.PutUint16(hdr[12:14], 1)
	binary.LittleEndian.PutUint16(hdr[14:16], 32)
	dib32.Write(hdr)
	dib32.Write([]byte{200, 150, 100, 255}) // B, G, R, A
	dib32.Write([]byte{0, 0, 0, 0})         // AND mask
	ico32 := makeICO([][]byte{dib32.Bytes()}, [][2]int{{1, 1}}, []int{32})
	encoded, err := NormalizeIcon(ico32)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := DecodePNG(encoded)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 100 || g>>8 != 150 || b>>8 != 200 || a>>8 != 255 {
		t.Fatalf("32-bit pixel=%d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}

	// 1x1 8-bit paletted
	var dib8 bytes.Buffer
	hdr8 := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr8[0:4], 40)
	binary.LittleEndian.PutUint32(hdr8[4:8], 1)
	binary.LittleEndian.PutUint32(hdr8[8:12], 2)
	binary.LittleEndian.PutUint16(hdr8[12:14], 1)
	binary.LittleEndian.PutUint16(hdr8[14:16], 8)
	binary.LittleEndian.PutUint32(hdr8[32:36], 2) // 2 palette entries
	dib8.Write(hdr8)
	dib8.Write([]byte{0, 0, 0, 0, 50, 100, 150, 0}) // palette[0], palette[1]
	dib8.Write([]byte{1, 0, 0, 0})                  // pixel index 1
	dib8.Write([]byte{0, 0, 0, 0})                  // AND mask
	ico8 := makeICO([][]byte{dib8.Bytes()}, [][2]int{{1, 1}}, []int{8})
	encoded, err = NormalizeIcon(ico8)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err = DecodePNG(encoded)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a = img.At(0, 0).RGBA()
	if r>>8 != 150 || g>>8 != 100 || b>>8 != 50 || a>>8 != 255 {
		t.Fatalf("8-bit pixel=%d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestNormalizeIconRejectsMalformedInputs(t *testing.T) {
	for _, sample := range [][]byte{
		nil,
		[]byte("not an image"),
		[]byte("<svg><script>alert(1)</script></svg>"),
		{0, 0, 1, 0, 1, 0}, // truncated ICO
		{0, 0, 1, 0, 1, 0, 16, 16, 0, 0, 1, 0, 32, 0, 99, 0, 0, 0, 22, 0, 0, 0}, // out of bounds ICO
	} {
		if _, err := NormalizeIcon(sample); err == nil {
			t.Fatalf("expected error for malformed sample %x", sample)
		}
	}
}
