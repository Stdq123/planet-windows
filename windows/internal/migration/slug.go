package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"windows/internal/backup"
)

// Remove a stale alias only in the private export copy, after proving that it
// belongs to this article and contains no unrecognized files.
func removeArchivedSlug(stage, slug, id string, attachments []string) error {
	if !backup.ValidArticleSlug(slug) || strings.EqualFold(slug, id) {
		return errors.New("原文章自定义路径无效")
	}
	dir := filepath.Join(stage, slug)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("原文章自定义路径目录无效")
	}
	path := filepath.Join(dir, "article.json")
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return errors.New("原文章自定义路径元数据无效")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var metadata struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &metadata) != nil || !strings.EqualFold(metadata.ID, id) {
		return errors.New("原文章自定义路径归属不匹配")
	}
	allowed := map[string]bool{"index.html": true, "article.json": true, "article.md": true, "_videothumbnail.png": true}
	for _, name := range attachments {
		if !backup.SafeName(name) {
			return errors.New("原附件名称无效")
		}
		allowed[strings.ToLower(name)] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || !allowed[strings.ToLower(entry.Name())] {
			return fmt.Errorf("原文章自定义路径包含未识别文件 %s", entry.Name())
		}
	}
	return os.RemoveAll(dir)
}
