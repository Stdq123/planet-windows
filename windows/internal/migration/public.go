package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"windows/internal/attachments"
	"windows/internal/backup"
	"windows/internal/sitegen"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

// StageImportedPublic rebuilds a supported Plain site from its saved data.
// Original HTML and private backup metadata are never copied into the public tree.
func StageImportedPublic(archive, destination string, raw json.RawMessage, site store.Site, articles []store.Article, templateDir string) error {
	if err := backup.CheckSimplePlain(raw, site, articles); err != nil {
		return err
	}
	if err := templateinstall.VerifyName(templateDir, "Plain"); err != nil {
		return fmt.Errorf("请选择已验证的 Plain 模板：%w", err)
	}
	var original struct {
		ID       string `json:"id"`
		Articles []struct {
			ID            string   `json:"id"`
			Attachments   []string `json:"attachments"`
			ArticleNumber *int     `json:"articleNumber"`
		} `json:"articles"`
	}
	if json.Unmarshal(raw, &original) != nil || !strings.EqualFold(original.ID, site.ID) {
		return errors.New("原备份元数据无效")
	}
	byID := map[string][]string{}
	numbers := map[string]*int{}
	for _, entry := range original.Articles {
		byID[strings.ToUpper(entry.ID)] = entry.Attachments
		numbers[strings.ToUpper(entry.ID)] = entry.ArticleNumber
	}
	generated := append([]store.Article{}, articles...)
	for i := range generated {
		a := &generated[i]
		names, ok := byID[strings.ToUpper(a.ID)]
		if !ok {
			return errors.New("原版文章标识无效")
		}
		a.Attachments = names
		a.ArticleNumber = numbers[strings.ToUpper(a.ID)]
		articleDir := filepath.Join(archive, a.ID)
		info, err := os.Lstat(articleDir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("原版文章目录无效")
		}
		for _, filename := range []string{"nft.json", "nft.json.cid.txt", "simple.html", "_cover.png", "_grid.jpg", "_videoThumbnail.png"} {
			if _, err := os.Lstat(filepath.Join(articleDir, filename)); err == nil {
				return fmt.Errorf("原文章包含 %s，暂不支持安全重建", filename)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		path := filepath.Join(articleDir, "article.json")
		info, err = os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() > 16<<20 {
				return errors.New("原公开文章元数据无效")
			}
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(b, &fields) != nil || fields == nil {
				return errors.New("原公开文章元数据无效")
			}
			if e = backup.CheckSimplePublicArticle(fields); e != nil {
				return e
			}
			if ref, ok := fields["articleReference"]; ok && string(ref) != "null" {
				var reference, publicID string
				if json.Unmarshal(ref, &reference) != nil || json.Unmarshal(fields["id"], &publicID) != nil || !strings.EqualFold(publicID, a.ID) {
					return errors.New("原文章引用标识无效")
				}
				a.ArticleReference = reference
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, name := range []string{"avatar.png", "podcastCoverArt.png", "favicon.ico"} {
		if _, err := os.Lstat(filepath.Join(archive, name)); err == nil {
			return fmt.Errorf("原站点包含 %s，暂不支持安全重建", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := sitegen.RenderWithTemplate(destination, site, generated, templateDir); err != nil {
		return err
	}
	for _, article := range generated {
		if err := attachments.CopyImported(filepath.Join(archive, article.ID), filepath.Join(destination, article.ID), article.Attachments); err != nil {
			return err
		}
	}
	return nil
}
