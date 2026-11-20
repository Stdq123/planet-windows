package siteimage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sort"
)

const (
	maxIconInputBytes  = 4 << 20
	maxIconOutputBytes = 512 << 10
	maxIconSide        = 128
	maxICOEntries      = 64
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// NormalizeIcon validates a remote or IPFS site icon (PNG, JPEG, GIF or ICO),
// downscales oversized images cleanly, and returns a base64-encoded PNG.
func NormalizeIcon(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxIconInputBytes {
		return "", errors.New("图标文件大小无效")
	}
	var img image.Image
	var err error
	if isICO(data) {
		img, err = decodeICO(data)
	} else {
		img, err = decode(data, false)
	}
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return "", errors.New("图标尺寸无效")
	}
	if w > maxIconSide || h > maxIconSide {
		dstW, dstH := fitIconSize(w, h, maxIconSide)
		img = downscaleIcon(img, dstW, dstH)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return "", err
	}
	if out.Len() > maxIconOutputBytes {
		return "", errors.New("转换后的图标过大")
	}
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}

func fitIconSize(w, h, limit int) (int, int) {
	if w >= h {
		dstH := h * limit / w
		if dstH < 1 {
			dstH = 1
		}
		return limit, dstH
	}
	dstW := w * limit / h
	if dstW < 1 {
		dstW = 1
	}
	return dstW, limit
}

func downscaleIcon(src image.Image, dstW, dstH int) *image.NRGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy0 := y * srcH / dstH
		sy1 := (y + 1) * srcH / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > srcH {
			sy1 = srcH
		}
		for x := 0; x < dstW; x++ {
			sx0 := x * srcW / dstW
			sx1 := (x + 1) * srcW / dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > srcW {
				sx1 = srcW
			}
			var sumR, sumG, sumB, sumA uint64
			var count uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, b, a := src.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					sumR += uint64(r)
					sumG += uint64(g)
					sumB += uint64(b)
					sumA += uint64(a)
					count++
				}
			}
			if count == 0 || sumA == 0 {
				dst.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			// Convert averaged premultiplied 16-bit RGBA back to non-premultiplied 8-bit NRGBA.
			a := uint8((sumA/count + 0x80) >> 8)
			r := uint8(minUint64((sumR*255+sumA/2)/sumA, 255))
			g := uint8(minUint64((sumG*255+sumA/2)/sumA, 255))
			b := uint8(minUint64((sumB*255+sumA/2)/sumA, 255))
			dst.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return dst
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func isICO(data []byte) bool {
	return len(data) >= 6 && binary.LittleEndian.Uint16(data[0:2]) == 0 && binary.LittleEndian.Uint16(data[2:4]) == 1
}

type icoEntry struct {
	w, h     int
	bitCount int
	payload  []byte
	isPNG    bool
}

func decodeICO(data []byte) (image.Image, error) {
	if !isICO(data) {
		return nil, errors.New("ICO 图标头无效")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count <= 0 || count > maxICOEntries || len(data) < 6+count*16 {
		return nil, errors.New("ICO 目录无效")
	}
	dirEnd := uint64(6 + count*16)
	entries := make([]icoEntry, 0, count)
	for i := 0; i < count; i++ {
		off := 6 + i*16
		w := int(data[off])
		if w == 0 {
			w = 256
		}
		h := int(data[off+1])
		if h == 0 {
			h = 256
		}
		bitCount := int(binary.LittleEndian.Uint16(data[off+6 : off+8]))
		size := uint64(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		imgOff := uint64(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		if size == 0 || imgOff < dirEnd || imgOff+size < imgOff || imgOff+size > uint64(len(data)) {
			continue
		}
		payload := data[imgOff : imgOff+size]
		entries = append(entries, icoEntry{
			w:        w,
			h:        h,
			bitCount: bitCount,
			payload:  payload,
			isPNG:    len(payload) >= len(pngSignature) && bytes.Equal(payload[:len(pngSignature)], pngSignature),
		})
	}
	if len(entries) == 0 {
		return nil, errors.New("ICO 不包含有效图像")
	}
	sort.SliceStable(entries, func(i, j int) bool {
		si := minInt(entries[i].w, entries[i].h)
		sj := minInt(entries[j].w, entries[j].h)
		if (si >= 32) != (sj >= 32) {
			return si >= 32
		}
		if si != sj {
			return si > sj
		}
		if entries[i].bitCount != entries[j].bitCount {
			return entries[i].bitCount > entries[j].bitCount
		}
		return entries[i].isPNG && !entries[j].isPNG
	})
	for _, entry := range entries {
		var img image.Image
		var err error
		if entry.isPNG {
			img, err = decode(entry.payload, true)
		} else {
			img, err = decodeICOBMP(entry.payload, entry.w, entry.h)
		}
		if err == nil {
			return img, nil
		}
	}
	return nil, errors.New("ICO 图标无法解码")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func decodeICOBMP(payload []byte, entryW, entryH int) (image.Image, error) {
	if len(payload) < 40 {
		return nil, errors.New("ICO BMP 头无效")
	}
	headerSize := int(binary.LittleEndian.Uint32(payload[0:4]))
	if headerSize < 40 || headerSize > len(payload) {
		return nil, errors.New("ICO BMP 头长度无效")
	}
	w := int(int32(binary.LittleEndian.Uint32(payload[4:8])))
	rawH := int(int32(binary.LittleEndian.Uint32(payload[8:12])))
	planes := binary.LittleEndian.Uint16(payload[12:14])
	bitCount := int(binary.LittleEndian.Uint16(payload[14:16]))
	compression := binary.LittleEndian.Uint32(payload[16:20])
	clrUsed := int(binary.LittleEndian.Uint32(payload[32:36]))
	if planes != 1 || compression != 0 {
		return nil, errors.New("ICO BMP 格式不受支持")
	}
	topDown := rawH < 0
	absH := rawH
	if absH < 0 {
		absH = -absH
	}
	if w <= 0 {
		w = entryW
	}
	h := entryH
	if absH == 2*entryH && entryH > 0 {
		h = entryH
	} else if absH > 0 && absH%2 == 0 {
		h = absH / 2
	} else if absH > 0 {
		h = absH
	}
	if w <= 0 || w > 256 || h <= 0 || h > 256 {
		return nil, errors.New("ICO BMP 尺寸无效")
	}
	var palette []color.NRGBA
	pixelOffset := headerSize
	switch bitCount {
	case 1, 4, 8:
		colors := clrUsed
		if colors == 0 {
			colors = 1 << bitCount
		}
		if colors <= 0 || colors > 256 || pixelOffset+colors*4 > len(payload) {
			return nil, errors.New("ICO BMP 调色板无效")
		}
		palette = make([]color.NRGBA, colors)
		for i := 0; i < colors; i++ {
			base := pixelOffset + i*4
			palette[i] = color.NRGBA{R: payload[base+2], G: payload[base+1], B: payload[base], A: 255}
		}
		pixelOffset += colors * 4
	case 24, 32:
		if clrUsed < 0 || clrUsed > 256 || pixelOffset+clrUsed*4 > len(payload) {
			return nil, errors.New("ICO BMP 颜色表无效")
		}
		pixelOffset += clrUsed * 4
	default:
		return nil, errors.New("ICO BMP 位深度不受支持")
	}
	xorRowBytes := ((w*bitCount + 31) / 32) * 4
	xorSize := xorRowBytes * h
	if pixelOffset+xorSize > len(payload) {
		return nil, errors.New("ICO BMP 像素数据不完整")
	}
	andOffset := pixelOffset + xorSize
	andRowBytes := ((w + 31) / 32) * 4
	hasAND := andOffset+andRowBytes*h <= len(payload)

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	hasAlpha := false
	for row := 0; row < h; row++ {
		dstY := row
		if !topDown {
			dstY = h - 1 - row
		}
		rowStart := pixelOffset + row*xorRowBytes
		for x := 0; x < w; x++ {
			var px color.NRGBA
			switch bitCount {
			case 32:
				base := rowStart + x*4
				px = color.NRGBA{R: payload[base+2], G: payload[base+1], B: payload[base], A: payload[base+3]}
				if px.A != 0 {
					hasAlpha = true
				}
			case 24:
				base := rowStart + x*3
				px = color.NRGBA{R: payload[base+2], G: payload[base+1], B: payload[base], A: 255}
			case 8:
				idx := int(payload[rowStart+x])
				if idx < len(palette) {
					px = palette[idx]
				}
			case 4:
				b := payload[rowStart+x/2]
				idx := int((b >> 4) & 0x0f)
				if x%2 == 1 {
					idx = int(b & 0x0f)
				}
				if idx < len(palette) {
					px = palette[idx]
				}
			case 1:
				idx := int((payload[rowStart+x/8] >> (7 - uint(x%8))) & 1)
				if idx < len(palette) {
					px = palette[idx]
				}
			}
			img.SetNRGBA(x, dstY, px)
		}
	}
	if bitCount < 32 || !hasAlpha {
		for row := 0; row < h; row++ {
			dstY := row
			if !topDown {
				dstY = h - 1 - row
			}
			for x := 0; x < w; x++ {
				px := img.NRGBAAt(x, dstY)
				if hasAND {
					mask := (payload[andOffset+row*andRowBytes+x/8] >> (7 - uint(x%8))) & 1
					if mask == 1 {
						px.A = 0
					} else {
						px.A = 255
					}
				} else {
					px.A = 255
				}
				img.SetNRGBA(x, dstY, px)
			}
		}
	}
	return img, nil
}
