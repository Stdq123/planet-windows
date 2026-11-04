package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
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
	return stageImportedPublic(archive, destination, raw, site, articles, templateDir, "")
}

func stageImportedPublic(archive, destination string, raw json.RawMessage, site store.Site, articles []store.Article, templateDir, marker string) error {
	var err error
	if err := backup.CheckSimplePlain(raw, site, articles); err != nil {
		return err
	}
	tags, articleTags, err := backup.ImportedTags(raw, articles)
	if err != nil {
		return err
	}
	site.Tags = tags
	if site.PlausibleOverride == nil {
		tracking, domain, server, err := backup.PlausiblePublicSettings(raw)
		if err != nil {
			return err
		}
		site.PlausibleEnabled, site.PlausibleDomain, site.PlausibleAPIServer = tracking, domain, server
	}
	site = store.ApplyPlausibleOverride(site)
	if err := templateinstall.Verify(templateDir); err != nil {
		return fmt.Errorf("请选择已验证的模板：%w", err)
	}
	var original struct {
		ID           string `json:"id"`
		Slug         string `json:"slug"`
		TemplateName string `json:"templateName"`
		Articles     []struct {
			ID            string   `json:"id"`
			Slug          string   `json:"slug"`
			Attachments   []string `json:"attachments"`
			ArticleNumber *int     `json:"articleNumber"`
			AudioFilename string   `json:"audioFilename"`
			VideoFilename string   `json:"videoFilename"`
		} `json:"articles"`
	}
	if json.Unmarshal(raw, &original) != nil || !strings.EqualFold(original.ID, site.ID) {
		return errors.New("原备份元数据无效")
	}
	byID := map[string][]string{}
	slugs := map[string]string{}
	numbers := map[string]*int{}
	originalAudio := map[string]string{}
	originalVideo := map[string]string{}
	for _, entry := range original.Articles {
		byID[strings.ToUpper(entry.ID)] = entry.Attachments
		slugs[strings.ToUpper(entry.ID)] = entry.Slug
		numbers[strings.ToUpper(entry.ID)] = entry.ArticleNumber
		originalAudio[strings.ToUpper(entry.ID)] = entry.AudioFilename
		originalVideo[strings.ToUpper(entry.ID)] = entry.VideoFilename
	}
	generated := append([]store.Article{}, articles...)
	for i := range generated {
		a := &generated[i]
		a.Tags = articleTags[strings.ToUpper(a.ID)]
		_, ok := byID[strings.ToUpper(a.ID)]
		if !ok {
			continue
		}
		a.Slug = slugs[strings.ToUpper(a.ID)]
		if a.ArticleNumber == nil {
			a.ArticleNumber = numbers[strings.ToUpper(a.ID)]
		}
		articleDir := filepath.Join(archive, a.ID)
		info, err := os.Lstat(articleDir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("原版文章目录无效")
		}
		for _, filename := range []string{"nft.json", "nft.json.cid.txt", "simple.html", "_cover.png", "_grid.jpg"} {
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
			for _, item := range []struct{ key, expected string }{{"audioFilename", originalAudio[strings.ToUpper(a.ID)]}, {"videoFilename", originalVideo[strings.ToUpper(a.ID)]}} {
				if value, ok := fields[item.key]; ok && string(value) != "null" {
					var actual string
					if json.Unmarshal(value, &actual) != nil || actual != item.expected {
						return errors.New("原公开媒体元数据与备份不一致")
					}
				}
			}
			if a.AudioFilename != "" && a.AudioFilename == originalAudio[strings.ToUpper(a.ID)] {
				owned := filepath.Join(filepath.Dir(filepath.Dir(archive)), "attachments", site.ID, a.ID, a.AudioFilename)
				if _, statErr := os.Lstat(owned); errors.Is(statErr, os.ErrNotExist) {
					if value, ok := fields["audioDuration"]; ok && string(value) != "null" {
						var duration int
						if json.Unmarshal(value, &duration) == nil && duration >= 0 {
							a.AudioDuration = &duration
						}
					}
				}
			}
			if ref, ok := fields["articleReference"]; ok && string(ref) != "null" {
				var reference, publicID string
				if json.Unmarshal(ref, &reference) != nil || json.Unmarshal(fields["id"], &publicID) != nil || !strings.EqualFold(publicID, a.ID) {
					return errors.New("原文章引用标识无效")
				}
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		a.ArticleReference = ""
		if a.ArticleNumber != nil && *a.ArticleNumber > 0 {
			prefix := strings.ToUpper(strings.TrimSpace(original.Slug))
			if prefix == "" {
				prefix = strings.ToUpper(site.ID)
				if len(prefix) > 8 {
					prefix = prefix[:8]
				}
			}
			a.ArticleReference = fmt.Sprintf("%s-%d", prefix, *a.ArticleNumber)
		}
	}
	root := filepath.Dir(filepath.Dir(archive))
	generated, err = attachments.EnrichMedia(root, site, generated)
	if err != nil {
		return err
	}
	for _, name := range []string{"podcastCoverArt.png"} {
		if _, err := os.Lstat(filepath.Join(archive, name)); err == nil {
			return fmt.Errorf("原站点包含 %s，暂不支持安全重建", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	metadataBytes, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		return err
	}
	var selected struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(metadataBytes, &selected) != nil || selected.Name == "" {
		return errors.New("模板信息无效")
	}
	settings, hasSettings, err := readTemplateSettings(archive, original.TemplateName, selected.Name)
	if err != nil {
		return err
	}
	if site.TemplateSettings != nil {
		values := map[string]json.RawMessage{}
		for k, v := range site.TemplateSettings {
			b, _ := json.Marshal(v)
			values[k] = b
		}
		settings, err = sitegen.NormalizeSettings(selected.Name, values)
		if err != nil {
			return err
		}
		hasSettings = true
	}
	for _, name := range []string{"avatar.png", "favicon.ico"} {
		if err := copyPublicSiteImage(archive, destination, name); err != nil {
			return err
		}
	}
	if err := sitegen.RenderWithTemplateSettingsForOwnership(destination, site, generated, templateDir, settings, marker); err != nil {
		return err
	}
	if hasSettings {
		b, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, "templateSettings.json"), append(b, '\n'), 0600); err != nil {
			return err
		}
	}
	for _, article := range generated {
		if article.VideoFilename != "" && article.VideoFilename == originalVideo[strings.ToUpper(article.ID)] {
			owned := filepath.Join(root, "attachments", site.ID, article.ID, article.VideoFilename)
			_, ownedErr := os.Lstat(owned)
			if errors.Is(ownedErr, os.ErrNotExist) {
				path := filepath.Join(archive, article.ID, "_videoThumbnail.png")
				if info, err := os.Lstat(path); err == nil {
					if !info.Mode().IsRegular() || info.Size() > 16<<20 {
						return errors.New("视频缩略图无效")
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					if err = os.WriteFile(filepath.Join(destination, article.ID, "_videoThumbnail.png"), data, 0600); err != nil {
						return err
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
		if err := attachments.CopyImportedCurrent(filepath.Dir(filepath.Dir(archive)), archive, destination, article); err != nil {
			return err
		}
		if err := attachments.GenerateVideoThumbnail(root, site, article, destination); err != nil {
			return err
		}
		if article.Slug != "" {
			alias := filepath.Join(destination, article.Slug)
			if err := os.Mkdir(alias, 0700); err != nil {
				return err
			}
			if err := backup.CopyTree(filepath.Join(destination, article.ID), alias); err != nil {
				return err
			}
		}
	}
	return nil
}

func LoadTemplateSettings(archive string, raw json.RawMessage, selectedName string) (map[string]string, error) {
	var original struct {
		TemplateName string `json:"templateName"`
	}
	if json.Unmarshal(raw, &original) != nil || original.TemplateName == "" {
		return nil, errors.New("原备份模板信息无效")
	}
	settings, _, err := readTemplateSettings(archive, original.TemplateName, selectedName)
	return settings, err
}

// Only verified settings are admitted into the generated public site.
func readTemplateSettings(archive, originalName, selectedName string) (map[string]string, bool, error) {
	path := filepath.Join(archive, "templateSettings.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, false, errors.New("原模板设置文件无效")
	}
	// Switching templates leaves the original archive intact but excludes the
	// previous template's settings from the newly generated public site.
	if originalName != selectedName {
		return nil, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, false, errors.New("原模板设置文件在读取时发生变化")
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, false, errors.New("原模板设置文件无效")
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(b, &settings) != nil || settings == nil {
		return nil, false, errors.New("原模板设置文件无效")
	}
	normalized, err := sitegen.NormalizeSettings(selectedName, settings)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

// These files are public site assets in an original Planet backup.
func copyPublicSiteImage(archive, destination, name string) error {
	path := filepath.Join(archive, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return fmt.Errorf("站点图片 %s 无效", name)
	}
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	actual, err := input.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return errors.New("站点图片在读取期间发生变化")
	}
	if name == "avatar.png" {
		if _, err := png.DecodeConfig(input); err != nil {
			return errors.New("站点头像不是有效的 PNG 图片")
		}
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	output, err := os.OpenFile(filepath.Join(destination, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	return err
}
