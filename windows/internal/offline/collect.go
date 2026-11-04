package offline

import (
	"errors"
	"os"
)

// Usage counts only regular files in managed snapshot directories.
type Usage struct {
	TotalBytes       int64 `json:"totalBytes"`
	ReclaimableBytes int64 `json:"reclaimableBytes"`
	ReclaimedBytes   int64 `json:"reclaimedBytes"`
	ManagedFiles     int   `json:"managedFiles"`
	ReclaimableFiles int   `json:"reclaimableFiles"`
	ReclaimedFiles   int   `json:"reclaimedFiles"`
}

// Collect must be serialized with snapshot creation, publication and readers.
// Unknown files and non-regular entries are never followed or removed.
func Collect(root string, references map[string]map[string]bool, clean bool) (Usage, error) {
	var usage Usage
	if _, err := mediaRoot(root); err != nil {
		return usage, err
	}
	base, err := os.OpenRoot(root)
	if err != nil {
		return usage, err
	}
	defer base.Close()
	before, err := base.Lstat("offline-media")
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return usage, errors.New("离线缓存目录无效")
	}
	cache, err := base.OpenRoot("offline-media")
	if err != nil {
		return usage, err
	}
	defer cache.Close()
	verify, err := cache.Open(".")
	if err != nil {
		return usage, err
	}
	current, err := verify.Stat()
	verify.Close()
	if err != nil || !os.SameFile(before, current) {
		return usage, errors.New("离线缓存目录已更改")
	}
	top, err := cache.Open(".")
	if err != nil {
		return usage, err
	}
	dirs, err := top.ReadDir(-1)
	top.Close()
	if err != nil {
		return usage, err
	}
	for _, entry := range dirs {
		if !snapshotName.MatchString(entry.Name()) {
			continue
		}
		info, e := cache.Lstat(entry.Name())
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return usage, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		dir, e := cache.OpenRoot(entry.Name())
		if e != nil {
			return usage, e
		}
		handle, e := dir.Open(".")
		if e != nil {
			dir.Close()
			return usage, e
		}
		opened, e := handle.Stat()
		if e != nil {
			handle.Close()
			dir.Close()
			return usage, e
		}
		if !os.SameFile(info, opened) {
			handle.Close()
			dir.Close()
			return usage, errors.New("离线缓存目录已更改")
		}
		files, e := handle.ReadDir(-1)
		handle.Close()
		if e != nil {
			dir.Close()
			return usage, e
		}
		for _, file := range files {
			if !fileName.MatchString(file.Name()) {
				continue
			}
			fi, e := dir.Lstat(file.Name())
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				dir.Close()
				return usage, e
			}
			if !fi.Mode().IsRegular() {
				continue
			}
			usage.TotalBytes += fi.Size()
			usage.ManagedFiles++
			if references[entry.Name()][file.Name()] {
				continue
			}
			usage.ReclaimableBytes += fi.Size()
			usage.ReclaimableFiles++
			if clean {
				if e = dir.Remove(file.Name()); e != nil {
					dir.Close()
					return usage, e
				}
				usage.ReclaimedBytes += fi.Size()
				usage.ReclaimedFiles++
			}
		}
		dir.Close()
		if clean {
			_ = cache.Remove(entry.Name())
		}
	}
	return usage, nil
}
