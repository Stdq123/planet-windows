//go:build !windows

package mediainfo

func windowsDuration(path string) *int { return nil }
