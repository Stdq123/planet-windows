// Package templateinstall validates and copies user-provided Planet templates.
package templateinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"windows/internal/backup"
	"windows/internal/sitegen"
	"windows/internal/store"
)

func Install(root, source string, site store.Site, articles []store.Article, library *store.Store) (string, error) {
	info, err := os.Lstat(filepath.Join(source, "template.json"))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", errors.New("缺少有效的 template.json")
	}
	b, err := os.ReadFile(filepath.Join(source, "template.json"))
	if err != nil {
		return "", err
	}
	var metadata struct {
		Name        string                     `json:"name"`
		BuildNumber int                        `json:"buildNumber"`
		Settings    map[string]json.RawMessage `json:"settings"`
		Ideal       int                        `json:"idealItemsPerPage"`
		Pagination  bool                       `json:"generateIndexPagination"`
		Archive     bool                       `json:"generateArchive"`
	}
	if err = json.Unmarshal(b, &metadata); err != nil || metadata.Name == "" {
		return "", errors.New("模板信息无效")
	}
	if site.Imported && metadata.Name != "Plain" {
		return "", errors.New("导入的 Plain 站点目前只能选择原版 Plain 模板")
	}
	parent := filepath.Join(root, "templates")
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, "template-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	if err = os.WriteFile(filepath.Join(stage, "template.json"), b, 0600); err != nil {
		return "", err
	}
	for _, name := range []string{"templates", "assets"} {
		from := filepath.Join(source, name)
		info, err := os.Lstat(from)
		if errors.Is(err, os.ErrNotExist) && name == "assets" {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("模板目录无效")
		}
		to := filepath.Join(stage, name)
		if err = os.Mkdir(to, 0700); err != nil {
			return "", err
		}
		if err = backup.CopyTree(from, to); err != nil {
			return "", err
		}
	}
	if err = Verify(stage); err != nil {
		return "", fmt.Errorf("当前仅支持已验证的 Plain、8-bit 和 Grid 原版模板：%w", err)
	}
	check, err := os.MkdirTemp(root, ".template-check-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(check)
	if len(articles) == 0 {
		now := time.Now().UTC()
		articles = []store.Article{{ID: "00000000-0000-4000-8000-000000000001", SiteID: site.ID, Title: "Preview", Content: "Preview", Created: now, Modified: now}}
	}
	if err = sitegen.RenderWithTemplate(check, site, articles, stage); err != nil {
		return "", err
	}
	if err = library.SetTemplate(site.ID, filepath.Base(stage), metadata.Name); err != nil {
		return "", err
	}
	keep = true
	return stage, nil
}
