// Package siteimage validates site avatars and produces the public icon files.
package siteimage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PresetAvatar struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Title    string `json:"title"`
	PNG      string `json:"png"`
}

// CircleCropPNG returns a square circular-masked PNG with transparent corners.
func CircleCropPNG(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	img, _, err := DecodePNG(value)
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	side := w
	if h < side {
		side = h
	}
	left := bounds.Min.X + (w-side)/2
	top := bounds.Min.Y + (h-side)/2
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	radius := float64(side) / 2.0
	r2 := radius * radius
	for y := 0; y < side; y++ {
		dy := float64(y) + 0.5 - radius
		for x := 0; x < side; x++ {
			dx := float64(x) + 0.5 - radius
			if dx*dx+dy*dy <= r2 {
				out.Set(x, y, img.At(left+x, top+y))
			} else {
				out.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// PresetAvatars returns a deterministic built-in gallery across the four macOS Planet categories.
func PresetAvatars() []PresetAvatar {
	specs := []struct {
		key, category, title string
		bg, fg, accent       color.NRGBA
		mode                 int
	}{
		{"NSTG-0001", "Nostalgia", "Maglev Pram", color.NRGBA{250, 238, 214, 255}, color.NRGBA{210, 77, 52, 255}, color.NRGBA{46, 98, 138, 255}, 0},
		{"NSTG-0020", "Nostalgia", "Hokusai", color.NRGBA{242, 236, 221, 255}, color.NRGBA{29, 78, 137, 255}, color.NRGBA{227, 181, 120, 255}, 1},
		{"NSTG-0043", "Nostalgia", "Tai chi", color.NRGBA{244, 240, 230, 255}, color.NRGBA{40, 44, 52, 255}, color.NRGBA{212, 93, 66, 255}, 2},
		{"NSTG-0061", "Nostalgia", "Carp", color.NRGBA{251, 235, 224, 255}, color.NRGBA{224, 90, 54, 255}, color.NRGBA{52, 130, 126, 255}, 3},
		{"MNCR-0001", "Monochrom", "Alliance", color.NRGBA{246, 246, 246, 255}, color.NRGBA{28, 28, 30, 255}, color.NRGBA{116, 116, 122, 255}, 0},
		{"MNCR-1001", "Monochrom", "Eye", color.NRGBA{32, 33, 36, 255}, color.NRGBA{242, 242, 244, 255}, color.NRGBA{138, 140, 148, 255}, 1},
		{"MNCR-2301", "Monochrom", "Moon", color.NRGBA{24, 26, 30, 255}, color.NRGBA{238, 238, 242, 255}, color.NRGBA{110, 114, 124, 255}, 2},
		{"MNCR-3201", "Monochrom", "Sakura", color.NRGBA{248, 248, 250, 255}, color.NRGBA{42, 42, 46, 255}, color.NRGBA{148, 148, 156, 255}, 3},
		{"PP-0001", "Pixel Planet", "Grey Glitch", color.NRGBA{36, 40, 52, 255}, color.NRGBA{96, 214, 236, 255}, color.NRGBA{244, 96, 122, 255}, 0},
		{"PP-0008", "Pixel Planet", "Nova", color.NRGBA{26, 22, 48, 255}, color.NRGBA{255, 184, 76, 255}, color.NRGBA{124, 92, 252, 255}, 1},
		{"PP-0013", "Pixel Planet", "Luna", color.NRGBA{22, 38, 66, 255}, color.NRGBA{132, 226, 255, 255}, color.NRGBA{255, 232, 142, 255}, 2},
		{"PP-0040", "Pixel Planet", "Matrix Green", color.NRGBA{16, 28, 22, 255}, color.NRGBA{64, 232, 128, 255}, color.NRGBA{24, 148, 82, 255}, 3},
		{"PN-0001", "Pixel Nomad", "PN-01", color.NRGBA{44, 34, 62, 255}, color.NRGBA{248, 198, 96, 255}, color.NRGBA{228, 92, 108, 255}, 0},
		{"PN-0012", "Pixel Nomad", "PN-12", color.NRGBA{28, 52, 68, 255}, color.NRGBA{112, 228, 196, 255}, color.NRGBA{246, 152, 86, 255}, 1},
		{"PN-0024", "Pixel Nomad", "PN-24", color.NRGBA{58, 32, 44, 255}, color.NRGBA{252, 146, 118, 255}, color.NRGBA{122, 212, 232, 255}, 2},
		{"PN-0042", "Pixel Nomad", "PN-42", color.NRGBA{32, 44, 36, 255}, color.NRGBA{188, 238, 116, 255}, color.NRGBA{252, 206, 92, 255}, 3},
	}
	out := make([]PresetAvatar, 0, len(specs))
	for _, spec := range specs {
		out = append(out, PresetAvatar{
			Key:      spec.key,
			Category: spec.category,
			Title:    spec.title,
			PNG:      renderPixelAvatar(spec.bg, spec.fg, spec.accent, spec.mode),
		})
	}
	return out
}

func PresetAvatarPNG(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	for _, item := range PresetAvatars() {
		if strings.EqualFold(item.Key, trimmed) {
			return item.PNG, nil
		}
	}
	return "", errors.New("未找到该内置头像")
}

func renderPixelAvatar(bg, fg, accent color.NRGBA, mode int) string {
	const side = 96
	const cells = 12
	const cellSize = side / cells
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		cy := y / cellSize
		for x := 0; x < side; x++ {
			cx := x / cellSize
			mx := cx
			if mx >= cells/2 {
				mx = cells - 1 - mx
			}
			c := bg
			dx := cx - cells/2
			dy := cy - cells/2
			switch mode {
			case 0:
				if (mx+cy)%3 == 0 && cx >= 2 && cx < 10 && cy >= 2 && cy < 10 {
					c = fg
				} else if mx == cy && cx >= 3 && cx < 9 {
					c = accent
				}
			case 1:
				dist := dx*dx + dy*dy
				if dist >= 6 && dist <= 18 {
					c = fg
				} else if dist < 6 {
					c = accent
				}
			case 2:
				if (cx >= 3 && cx <= 8 && (cy == 3 || cy == 8)) || (cy >= 3 && cy <= 8 && (cx == 3 || cx == 8)) {
					c = fg
				} else if cx >= 5 && cx <= 6 && cy >= 5 && cy <= 6 {
					c = accent
				}
			default:
				if (mx == 2 && cy >= 3 && cy <= 8) || (cy == 4 && mx >= 2 && mx <= 4) || (cy == 7 && mx >= 3 && mx <= 5) {
					c = fg
				} else if (mx+cy)%4 == 1 && cy >= 2 && cy <= 9 {
					c = accent
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

const maxBytes = 8 << 20
const maxPixels = 16 * 1024 * 1024

func decode(data []byte, pngOnly bool) (image.Image, error) {
	if len(data) == 0 || len(data) > maxBytes {
		return nil, errors.New("头像文件大小无效")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (pngOnly && format != "png") || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > maxPixels {
		return nil, errors.New("头像格式或尺寸无效")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("头像图片无法解码")
	}
	return img, nil
}

// DecodePNG validates a stored base64-encoded PNG and bounds decoded memory.
func DecodePNG(value string) (image.Image, []byte, error) {
	if len(value) > base64.StdEncoding.EncodedLen(maxBytes) {
		return nil, nil, errors.New("头像文件过大")
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, nil, errors.New("头像编码无效")
	}
	img, err := decode(data, true)
	return img, data, err
}

// ReadAvatar converts a selected PNG, JPEG or GIF into a bounded PNG.
func ReadAvatar(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", errors.New("请选择不大于 8 MiB 的图片文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return "", errors.New("读取时图片文件发生变化")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", err
	}
	img, err := decode(data, false)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err = png.Encode(&out, img); err != nil {
		return "", err
	}
	if out.Len() > maxBytes {
		return "", errors.New("转换后的头像超过 8 MiB")
	}
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}

// WriteFiles writes a PNG avatar and a 64px ICO with an embedded PNG.
func WriteFiles(dir, value string) error {
	if value == "" {
		return nil
	}
	img, data, err := DecodePNG(value)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "avatar.png"), data, 0600); err != nil {
		return err
	}
	const side = 64
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	crop := w
	if h < crop {
		crop = h
	}
	left := bounds.Min.X + (w-crop)/2
	top := bounds.Min.Y + (h-crop)/2
	icon := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			icon.Set(x, y, img.At(left+x*crop/side, top+y*crop/side))
		}
	}
	var pixels bytes.Buffer
	if err = png.Encode(&pixels, icon); err != nil {
		return err
	}
	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], 1)
	header[6], header[7] = side, side
	binary.LittleEndian.PutUint16(header[10:12], 1)
	binary.LittleEndian.PutUint16(header[12:14], 32)
	binary.LittleEndian.PutUint32(header[14:18], uint32(pixels.Len()))
	binary.LittleEndian.PutUint32(header[18:22], 22)
	return os.WriteFile(filepath.Join(dir, "favicon.ico"), append(header, pixels.Bytes()...), 0600)
}
