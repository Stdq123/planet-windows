package sitegen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"windows/internal/backup"
	"windows/internal/preview"
	"windows/internal/store"
	"windows/internal/templates"
)

// RenderWithTemplate implements the index, article and archive subset of Planet templates.
func RenderWithTemplate(dir string, site store.Site, articles []store.Article, templateDir string) error {
	var metadata struct {
		Name       string `json:"name"`
		Ideal      int    `json:"idealItemsPerPage"`
		Pagination bool   `json:"generateIndexPagination"`
		Archive    bool   `json:"generateArchive"`
	}
	b, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &metadata); err != nil || metadata.Name == "" {
		return errors.New("模板信息无效")
	}
	if err := Render(dir, site, articles); err != nil {
		return err
	}
	assets := filepath.Join(templateDir, "assets")
	if info, err := os.Stat(assets); err == nil && info.IsDir() {
		if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
			return err
		}
		if err := backup.CopyTree(assets, filepath.Join(dir, "assets")); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cssHash := ""
	if css, err := os.ReadFile(filepath.Join(dir, "assets", "style.css")); err == nil {
		hash := sha256.Sum256(css)
		cssHash = hex.EncodeToString(hash[:])
	}
	planet := map[string]any{"id": site.ID, "name": site.Name, "about": site.About, "ipns": site.IPNS, "created": site.Created, "tags": map[string]string{}}
	items := append([]store.Article{}, articles...)
	sort.Slice(items, func(i, j int) bool { return items[i].Created.After(items[j].Created) })
	rows := make([]map[string]any, 0, len(items))
	byID := map[string]map[string]any{}
	archive := map[string][]map[string]any{}
	sections := []string{}
	for _, a := range items {
		html, err := preview.Markdown(a.Content)
		if err != nil {
			return err
		}
		row := map[string]any{"id": a.ID, "link": "/" + a.ID + "/", "title": a.Title, "content": a.Content, "contentRendered": html, "created": a.Created, "modified": a.Modified, "attachments": a.Attachments, "externalLink": "", "slug": "", "hasVideo": false, "hasAudio": false, "heroImage": nil, "tags": map[string]string{}}
		rows = append(rows, row)
		byID[a.ID] = row
		year := strconv.Itoa(a.Created.Year())
		if _, ok := archive[year]; !ok {
			sections = append(sections, year)
		}
		archive[year] = append(archive[year], row)
	}
	aboutHTML, err := preview.Markdown(site.About)
	if err != nil {
		return err
	}
	context := map[string]any{"planet": planet, "articles": rows, "assets_prefix": "./", "page_title": site.Name, "page_description": site.About, "page_description_html": aboutHTML, "article_summary": "", "current_item_type": "index", "site_navigation": []any{}, "template_settings": map[string]any{}, "style_css_sha256": cssHash, "custom_code_head": "", "custom_code_body_start": "", "custom_code_body_end": "", "total_pages": 1, "current_page": 1, "has_avatar": false, "has_podcast": false}
	templatesDir := filepath.Join(templateDir, "templates")
	perPage := len(rows)
	if metadata.Pagination && metadata.Ideal > 0 {
		perPage = metadata.Ideal
	}
	if perPage == 0 {
		perPage = 1
	}
	totalPages := (len(rows) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	for page := 1; page <= totalPages; page++ {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		start := (page - 1) * perPage
		end := start + perPage
		if end > len(rows) {
			end = len(rows)
		}
		ctx["articles"] = rows[start:end]
		ctx["current_page"] = page
		ctx["total_pages"] = totalPages
		if page > 1 {
			ctx["previous_page"] = fmt.Sprintf("page%d.html", page-1)
		}
		if page < totalPages {
			ctx["next_page"] = fmt.Sprintf("page%d.html", page+1)
		}
		index, err := templates.Render(templatesDir, "index.html", ctx)
		if err != nil {
			return fmt.Errorf("模板首页渲染失败：%w", err)
		}
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("page%d.html", page)), []byte(index), 0600); err != nil {
			return err
		}
		if page == 1 {
			if err = os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0600); err != nil {
				return err
			}
		}
	}
	for _, a := range items {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		ctx["assets_prefix"] = "../"
		ctx["page_title"] = a.Title
		ctx["current_item_type"] = "article"
		ctx["article"] = byID[a.ID]
		ctx["article_id"] = a.ID
		ctx["article_type"] = 0
		ctx["content_html"] = byID[a.ID]["contentRendered"]
		page, err := templates.Render(templatesDir, "blog.html", ctx)
		if err != nil {
			return fmt.Errorf("模板文章渲染失败：%w", err)
		}
		if err = os.WriteFile(filepath.Join(dir, a.ID, "index.html"), []byte(page), 0600); err != nil {
			return err
		}
	}
	if metadata.Archive {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		ctx["page_title"] = site.Name + " - Archive"
		ctx["current_item_type"] = "archive"
		ctx["archive"] = archive
		ctx["archive_sections"] = sections
		page, err := templates.Render(templatesDir, "archive.html", ctx)
		if err != nil {
			return fmt.Errorf("模板归档渲染失败：%w", err)
		}
		if err = os.WriteFile(filepath.Join(dir, "archive.html"), []byte(page), 0600); err != nil {
			return err
		}
	}
	return nil
}
