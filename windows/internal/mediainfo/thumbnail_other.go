//go:build !windows

package mediainfo

import "errors"

func VideoThumbnail(path string) ([]byte, error) {
	return nil, errors.New("video thumbnails are unavailable on this platform")
}
