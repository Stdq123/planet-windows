// Package siteimage validates site avatars and produces the public icon files.
package siteimage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

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
