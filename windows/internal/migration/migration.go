// Package migration imports and exports Planet backup directories.
package migration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"windows/internal/attachments"
	"windows/internal/backup"
	"windows/internal/ipfs"
	"windows/internal/sitegen"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

type Manager struct {
	mu               sync.Mutex
	root, executable string
	store            *store.Store
	node             *ipfs.Client
}

var generatedPageName = regexp.MustCompile(`^page[1-9][0-9]*\.html$`)

func New(root, executable string, library *store.Store, node *ipfs.Client) *Manager {
	return &Manager{root: root, executable: executable, store: library, node: node}
}

func (m *Manager) Import(source string) (store.Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !strings.EqualFold(filepath.Ext(source), ".planet") {
		return store.Site{}, errors.New("请选择 .planet 备份目录")
	}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return store.Site{}, err
	}
	stage, err := os.MkdirTemp(m.root, ".planet-import-")
	if err != nil {
		return store.Site{}, err
	}
	defer os.RemoveAll(stage)
	copyDir := filepath.Join(stage, "source.planet")
	if err := os.Mkdir(copyDir, 0700); err != nil {
		return store.Site{}, err
	}
	if err := backup.CopyWithKey(source, copyDir); err != nil {
		return store.Site{}, err
	}
	parsed, err := backup.Read(copyDir)
	if err != nil {
		return store.Site{}, err
	}
	sites, err := m.store.ListSites()
	if err != nil {
		return store.Site{}, err
	}
	for _, site := range sites {
		if site.ID == parsed.Site.ID {
			return store.Site{}, errors.New("站点已经存在")
		}
	}
	// Validate the key and its identity in a throwaway repository first.
	validation := ipfs.New(m.executable, filepath.Join(stage, "key-check"))
	if err := validation.Init(); err != nil {
		return store.Site{}, err
	}
	canonical, err := validation.ImportKey(parsed.Site.ID, filepath.Join(copyDir, "planet.key"))
	if err != nil {
		return store.Site{}, fmt.Errorf("私钥无效：%w", err)
	}
	matches, err := validation.KeyMatches(parsed.Site.ID, parsed.Site.IPNS)
	if err != nil || !matches {
		return store.Site{}, errors.New("备份私钥与 IPNS 身份不匹配")
	}
	archiveParent := filepath.Join(m.root, "imports")
	if err := os.MkdirAll(archiveParent, 0700); err != nil {
		return store.Site{}, err
	}
	archive := filepath.Join(archiveParent, parsed.Site.ID)
	if _, err := os.Lstat(archive); err == nil {
		return store.Site{}, errors.New("站点归档目录已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return store.Site{}, err
	}
	archiveTemp, err := os.MkdirTemp(archiveParent, ".import-")
	if err != nil {
		return store.Site{}, err
	}
	defer os.RemoveAll(archiveTemp)
	if err := backup.CopyTree(copyDir, archiveTemp); err != nil {
		return store.Site{}, err
	}
	if err := os.Rename(archiveTemp, archive); err != nil {
		return store.Site{}, err
	}
	keepArchive := false
	defer func() {
		if !keepArchive {
			_ = os.RemoveAll(archive)
		}
	}()
	existing, err := m.node.LookupKey(parsed.Site.ID)
	if err != nil {
		return store.Site{}, err
	}
	createdKey := false
	if existing == "" {
		actual, e := m.node.ImportKey(parsed.Site.ID, filepath.Join(copyDir, "planet.key"))
		if e != nil {
			return store.Site{}, e
		}
		createdKey = true
		if actual != canonical {
			_ = m.node.RemoveKey(parsed.Site.ID)
			return store.Site{}, errors.New("导入的密钥身份不匹配")
		}
	} else if existing != canonical {
		return store.Site{}, errors.New("已有同名密钥与备份不匹配")
	}
	parsed.Site.IPNS = canonical
	if err := m.store.Import(parsed.Site, parsed.Articles, parsed.Raw); err != nil {
		if createdKey {
			_ = m.node.RemoveKey(parsed.Site.ID)
		}
		return store.Site{}, err
	}
	keepArchive = true
	return parsed.Site, nil
}

func (m *Manager) Export(siteID, parent string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, articles, raw, err := m.store.PublicationSnapshot(siteID)
	if err != nil {
		return "", err
	}
	site := &snapshot
	rebuild := false
	if len(raw) > 0 {
		if unchangedErr := backup.CheckUnchanged(raw, *site, articles); unchangedErr != nil {
			if site.TemplateID == "" {
				return "", unchangedErr
			}
			if err := backup.CheckSimplePlain(raw, *site, articles); err != nil {
				return "", err
			}
			if !strings.HasPrefix(site.TemplateID, "template-") || !backup.SafeName(site.TemplateID) {
				return "", errors.New("模板路径无效")
			}
			if err := templateinstall.Verify(filepath.Join(m.root, "templates", site.TemplateID)); err != nil {
				return "", err
			}
			for _, name := range []string{"podcastCoverArt.png"} {
				if _, err := os.Lstat(filepath.Join(m.root, "imports", siteID, name)); err == nil {
					return "", fmt.Errorf("原站点包含 %s，暂不支持安全重建", name)
				} else if !errors.Is(err, os.ErrNotExist) {
					return "", err
				}
			}
			for _, article := range articles {
				for _, name := range []string{"nft.json", "nft.json.cid.txt", "simple.html", "_cover.png", "_grid.jpg"} {
					if _, err := os.Lstat(filepath.Join(m.root, "imports", siteID, article.ID, name)); err == nil {
						return "", fmt.Errorf("原文章包含 %s，暂不支持安全重建", name)
					} else if !errors.Is(err, os.ErrNotExist) {
						return "", err
					}
				}
			}
			rebuild = true
		}
	}
	if site.IPNS == "" {
		identity, e := m.node.EnsureKey(siteID)
		if e != nil {
			return "", e
		}
		if e = m.store.SetIdentity(siteID, identity); e != nil {
			return "", e
		}
		site.IPNS = identity
	}
	match, err := m.node.KeyMatches(siteID, site.IPNS)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("站点密钥缺失或身份不匹配")
	}
	stage, err := os.MkdirTemp(parent, "Planet-export-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if len(raw) > 0 {
		if err := backup.CopyTree(filepath.Join(m.root, "imports", siteID), stage); err != nil {
			return "", err
		}
		if rebuild {
			if err := rejectUnregisteredArchivedFiles(filepath.Join(m.root, "imports", siteID), raw, articles); err != nil {
				return "", err
			}
			generated, err := os.MkdirTemp(m.root, ".planet-render-")
			if err != nil {
				return "", err
			}
			defer os.RemoveAll(generated)
			if err := StageImportedPublic(filepath.Join(m.root, "imports", siteID), generated, raw, *site, articles, filepath.Join(m.root, "templates", site.TemplateID)); err != nil {
				return "", err
			}
			// Do not retain old settings when the newly selected template has none.
			if _, err := os.Lstat(filepath.Join(generated, "templateSettings.json")); errors.Is(err, os.ErrNotExist) {
				path := filepath.Join(stage, "templateSettings.json")
				if info, e := os.Lstat(path); e == nil {
					if !info.Mode().IsRegular() {
						return "", errors.New("旧模板设置文件无效")
					}
					if e = os.Remove(path); e != nil {
						return "", e
					}
				} else if !errors.Is(e, os.ErrNotExist) {
					return "", e
				}
			} else if err != nil {
				return "", err
			}
			if err := rejectUnrecognizedAssets(filepath.Join(stage, "assets"), filepath.Join(generated, "assets")); err != nil {
				return "", err
			}
			// A source page is safe to replace only if it matches either the
			// current output or a faithful regeneration of the original snapshot.
			originalPages, err := os.MkdirTemp(m.root, ".planet-original-pages-")
			if err != nil {
				return "", err
			}
			defer os.RemoveAll(originalPages)
			if err := renderOriginalTagPages(filepath.Join(m.root, "imports", siteID), originalPages, raw, filepath.Join(m.root, "templates", site.TemplateID), ""); err != nil {
				originalPages = ""
			}
			var originalInfo struct {
				TemplateName string `json:"templateName"`
			}
			var markedPages, marker string
			if originalPages != "" && json.Unmarshal(raw, &originalInfo) == nil && originalInfo.TemplateName == "Sepia" {
				var nonce [16]byte
				if _, err := rand.Read(nonce[:]); err == nil {
					marker = "PLANETTIMESTAMP" + hex.EncodeToString(nonce[:]) + "END"
					markedPages, err = os.MkdirTemp(m.root, ".planet-original-marker-")
					if err != nil {
						return "", err
					}
					defer os.RemoveAll(markedPages)
					if err := renderOriginalTagPages(filepath.Join(m.root, "imports", siteID), markedPages, raw, filepath.Join(m.root, "templates", site.TemplateID), marker); err != nil {
						markedPages = ""
					}
				}
			}
			if err := removeObsoleteImportedFiles(stage, raw, articles, generated, originalPages, markedPages, marker); err != nil {
				return "", err
			}
			for _, article := range articles {
				thumb := filepath.Join(generated, article.ID, "_videoThumbnail.png")
				if _, e := os.Lstat(thumb); errors.Is(e, os.ErrNotExist) {
					if e = os.Remove(filepath.Join(stage, article.ID, "_videoThumbnail.png")); e != nil && !errors.Is(e, os.ErrNotExist) {
						return "", e
					}
				} else if e != nil {
					return "", e
				}
				oldPath := filepath.Join(stage, article.ID, "article.json")
				if err := mergeArticleMetadata(oldPath, filepath.Join(generated, article.ID, "article.json"), article.ID); err != nil {
					return "", err
				}
			}
			entries, err := os.ReadDir(stage)
			if err != nil {
				return "", err
			}
			for _, entry := range entries {
				if !generatedPageName.MatchString(entry.Name()) {
					continue
				}
				if _, err := os.Lstat(filepath.Join(generated, entry.Name())); errors.Is(err, os.ErrNotExist) {
					info, err := entry.Info()
					if err != nil || !info.Mode().IsRegular() {
						return "", errors.New("旧分页文件无效")
					}
					if err = os.Remove(filepath.Join(stage, entry.Name())); err != nil {
						return "", err
					}
				} else if err != nil {
					return "", err
				}
			}
			if err := backup.OverlayTree(generated, stage); err != nil {
				return "", err
			}
		}
		for _, article := range articles {
			if err := os.MkdirAll(filepath.Join(stage, article.ID), 0700); err != nil {
				return "", err
			}
		}
	} else {
		articles, err = attachments.EnrichMedia(m.root, *site, articles)
		if err != nil {
			return "", err
		}
		if site.TemplateID != "" {
			if !strings.HasPrefix(site.TemplateID, "template-") || !backup.SafeName(site.TemplateID) {
				return "", errors.New("模板路径无效")
			}
			templateDir := filepath.Join(m.root, "templates", site.TemplateID)
			if err = templateinstall.Verify(templateDir); err != nil {
				return "", err
			}
			err = sitegen.RenderWithTemplate(stage, *site, articles, templateDir)
		} else {
			err = sitegen.Render(stage, *site, articles)
		}
		if err != nil {
			return "", err
		}
		if err := attachments.CopyToSite(m.root, stage, articles); err != nil {
			return "", err
		}
		for _, article := range articles {
			if err := attachments.GenerateVideoThumbnail(m.root, *site, article, stage); err != nil {
				return "", err
			}
		}
		if err := sitegen.WriteAliases(stage, articles); err != nil {
			return "", err
		}
	}
	var metadata []byte
	if len(raw) > 0 && !rebuild {
		// Number assignment happens in the library, but an untouched backup's
		// original public files still correspond to the original metadata.
		metadata = append([]byte(nil), raw...)
	} else {
		metadata, err = backup.Merge(raw, *site, articles)
		if err != nil {
			return "", err
		}
	}
	// The saved publication may predate later edits or deletions. Backups rebuilt
	// in Windows must not instruct the Mac app to republish that older CID.
	if rebuild || site.LastPublishedCID != "" {
		var rebuilt map[string]json.RawMessage
		if err := json.Unmarshal(metadata, &rebuilt); err != nil || rebuilt == nil {
			return "", errors.New("重建的备份元数据无效")
		}
		delete(rebuilt, "lastPublishedCID")
		delete(rebuilt, "lastPublished")
		metadata, err = json.MarshalIndent(rebuilt, "", "  ")
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "planet.json"), metadata, 0600); err != nil {
		return "", err
	}
	if err := m.node.ExportKey(siteID, filepath.Join(stage, "planet.key")); err != nil {
		return "", err
	}
	final := stage + ".planet"
	if _, err := os.Lstat(final); err == nil {
		return "", errors.New("备份目标已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(stage, final); err != nil {
		return "", err
	}
	return final, nil
}

// Mac restores the complete assets directory. Do not carry unknown old files
// into a rebuilt site; the immutable source backup remains available to inspect.
func rejectUnrecognizedAssets(oldDir, generatedDir string) error {
	if _, err := os.Lstat(oldDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(oldDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == oldDir || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(oldDir, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(generatedDir, rel))
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("原模板包含未识别文件 %s；为避免恢复后意外公开，无法重建备份", rel)
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("生成的模板文件无效")
		}
		return nil
	})
}

// Mac imports copy each complete article directory into the public site.
// Preserve the source archive and reject a rebuild that would retain files whose
// purpose cannot be determined instead of silently publishing or deleting them.
func rejectUnregisteredArchivedFiles(archive string, raw json.RawMessage, articles []store.Article) error {
	var original struct {
		Articles []struct {
			ID          string   `json:"id"`
			Attachments []string `json:"attachments"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &original); err != nil {
		return err
	}
	kept := make(map[string]bool, len(articles))
	for _, article := range articles {
		kept[strings.ToUpper(article.ID)] = true
	}
	for _, article := range original.Articles {
		if !kept[strings.ToUpper(article.ID)] {
			continue
		}
		if !backup.SafeName(article.ID) {
			return errors.New("原文章标识无效")
		}
		allowed := map[string]bool{"index.html": true, "article.json": true, "article.md": true, "_videothumbnail.png": true}
		for _, name := range article.Attachments {
			if !backup.SafeName(name) {
				return errors.New("原附件名称无效")
			}
			allowed[strings.ToLower(name)] = true
		}
		entries, err := os.ReadDir(filepath.Join(archive, article.ID))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !allowed[strings.ToLower(entry.Name())] {
				return fmt.Errorf("原文章包含未登记文件 %s；为避免恢复后意外公开，无法重建备份", entry.Name())
			}
		}
	}
	return nil
}

// Preserve unrecognized public metadata while replacing the fields we rebuilt.
func mergeArticleMetadata(oldPath, newPath, id string) error {
	info, err := os.Lstat(oldPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return errors.New("原文章元数据无效")
	}
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	newBytes, err := os.ReadFile(newPath)
	if err != nil {
		return err
	}
	var old, newer map[string]json.RawMessage
	if json.Unmarshal(oldBytes, &old) != nil || old == nil || json.Unmarshal(newBytes, &newer) != nil || newer == nil {
		return errors.New("原文章元数据无效")
	}
	if err := backup.CheckSimplePublicArticle(old); err != nil {
		return err
	}
	if raw, ok := old["id"]; ok {
		var existing string
		if json.Unmarshal(raw, &existing) != nil || !strings.EqualFold(existing, id) {
			return errors.New("原文章元数据标识不匹配")
		}
	}
	// Generated optional fields must replace old values even when omitted.
	for _, key := range []string{"slug", "tags", "attachments", "audioFilename", "videoFilename", "audioDuration", "audioByteLength", "hasAudio", "hasVideo"} {
		delete(old, key)
	}
	for key, value := range newer {
		old[key] = value
	}
	merged, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(newPath, merged, 0600)
}

// Remove only entries deliberately removed from the imported site, inside the private staging copy.
func removeObsoleteImportedFiles(stage string, raw json.RawMessage, articles []store.Article, generated ...string) error {
	var original struct {
		TemplateName string            `json:"templateName"`
		Tags         map[string]string `json:"tags"`
		Articles     []struct {
			ID          string            `json:"id"`
			Slug        string            `json:"slug"`
			Attachments []string          `json:"attachments"`
			Tags        map[string]string `json:"tags"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &original); err != nil {
		return err
	}
	if _, _, err := backup.ImportedTags(raw, articles); err != nil {
		return err
	}
	tagFiles := map[string]bool{"tags.html": true}
	for key := range original.Tags {
		tagFiles[key+".html"] = true
	}
	for _, article := range original.Articles {
		for key := range article.Tags {
			tagFiles[key+".html"] = true
		}
	}
	for _, article := range articles {
		for key := range article.Tags {
			tagFiles[key+".html"] = true
		}
	}
	var obsoleteTagPages []string
	for name := range tagFiles {
		path := filepath.Join(stage, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("原标签页面无效")
		}
		if len(generated) == 0 || generated[0] == "" {
			return fmt.Errorf("无法确认原标签页面 %s 的归属，不能重建备份", name)
		}
		current, _ := sameRegularFiles(path, filepath.Join(generated[0], name))
		old := false
		if !current && len(generated) > 1 && generated[1] != "" {
			old, _ = sameRegularFiles(path, filepath.Join(generated[1], name))
			if !old && original.TemplateName == "Sepia" && len(generated) > 3 && generated[2] != "" && generated[3] != "" {
				old = sameSepiaTimestampPage(path, filepath.Join(generated[1], name), filepath.Join(generated[2], name), generated[3])
			}
		}
		if !current && !old {
			return fmt.Errorf("无法确认原标签页面 %s 的归属，不能重建备份", name)
		}
		if !current {
			if _, err := os.Lstat(filepath.Join(generated[0], name)); errors.Is(err, os.ErrNotExist) {
				obsoleteTagPages = append(obsoleteTagPages, path)
			} else if err != nil {
				return err
			}
		}
	}
	for _, path := range obsoleteTagPages {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	current := map[string]store.Article{}
	for _, article := range articles {
		current[strings.ToUpper(article.ID)] = article
	}
	for _, old := range original.Articles {
		if !backup.SafeName(old.ID) {
			return errors.New("原文章标识无效")
		}
		if old.Slug != "" {
			if err := removeArchivedSlug(stage, old.Slug, old.ID, old.Attachments); err != nil {
				return err
			}
		}
		article, ok := current[strings.ToUpper(old.ID)]
		dir := filepath.Join(stage, strings.ToUpper(old.ID))
		if !ok {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			continue
		}
		keep := map[string]bool{}
		for _, name := range article.Attachments {
			keep[strings.ToLower(name)] = true
		}
		for _, name := range old.Attachments {
			if !backup.SafeName(name) {
				return errors.New("原附件名称无效")
			}
			if !keep[strings.ToLower(name)] {
				if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
	}
	return nil
}
