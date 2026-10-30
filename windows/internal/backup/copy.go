package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CopyTree copies a backup into an owned empty staging directory without its key.
func CopyTree(source, destination string) error {
	return copyTree(source, destination, false)
}

// CopyWithKey is for a temporary private staging directory, never an archive.
func CopyWithKey(source, destination string) error {
	return copyTree(source, destination, true)
}

func copyTree(source, destination string, includeKey bool) error {
	seen := map[string]bool{}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if !SafeName(entry.Name()) {
			return fmt.Errorf("不支持的备份文件名：%s", rel)
		}
		folded := strings.ToLower(rel)
		if seen[folded] {
			return fmt.Errorf("备份存在大小写冲突：%s", rel)
		}
		seen[folded] = true
		if !includeKey && strings.EqualFold(rel, "planet.key") {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("备份包含不支持的链接或特殊文件：%s", rel)
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.Mkdir(target, 0700)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		opened, err := input.Stat()
		if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			return errors.New("备份文件在复制期间发生变化")
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		return err
	})
}

// OverlayTree replaces generated files in an owned staging directory while
// retaining unrelated files; neither source nor destination may contain links.
func OverlayTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if !SafeName(entry.Name()) || strings.EqualFold(rel, "planet.key") {
			return errors.New("生成文件路径无效")
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("生成文件无效")
		}
		target := filepath.Join(destination, rel)
		existing, statErr := os.Lstat(target)
		if info.IsDir() {
			if errors.Is(statErr, os.ErrNotExist) {
				return os.Mkdir(target, 0700)
			}
			if statErr != nil || !existing.IsDir() || existing.Mode()&os.ModeSymlink != 0 {
				return errors.New("目标目录无效")
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("生成文件无效")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if statErr == nil && !existing.Mode().IsRegular() {
			return errors.New("目标文件无效")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		return err
	})
}
