// Package migration imports and exports Planet backup directories.
package migration

import (
	"encoding/json"
	"errors"
	"fmt"
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
	sites, err := m.store.ListSites()
	if err != nil {
		return "", err
	}
	var site *store.Site
	for i := range sites {
		if sites[i].ID == siteID {
			site = &sites[i]
			break
		}
	}
	if site == nil {
		return "", errors.New("站点不存在")
	}
	articles, err := m.store.ListArticles(siteID)
	if err != nil {
		return "", err
	}
	raw, err := m.store.BackupMetadata(siteID)
	if err != nil {
		return "", err
	}
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
			if err := templateinstall.VerifyName(filepath.Join(m.root, "templates", site.TemplateID), "Plain"); err != nil {
				return "", err
			}
			for _, name := range []string{"avatar.png", "podcastCoverArt.png", "favicon.ico"} {
				if _, err := os.Lstat(filepath.Join(m.root, "imports", siteID, name)); err == nil {
					return "", fmt.Errorf("原站点包含 %s，暂不支持安全重建", name)
				} else if !errors.Is(err, os.ErrNotExist) {
					return "", err
				}
			}
			for _, article := range articles {
				for _, name := range []string{"nft.json", "nft.json.cid.txt", "simple.html", "_cover.png", "_grid.jpg", "_videoThumbnail.png"} {
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
			generated, err := os.MkdirTemp(m.root, ".planet-render-")
			if err != nil {
				return "", err
			}
			defer os.RemoveAll(generated)
			if err := StageImportedPublic(filepath.Join(m.root, "imports", siteID), generated, raw, *site, articles, filepath.Join(m.root, "templates", site.TemplateID)); err != nil {
				return "", err
			}
			for _, article := range articles {
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
	}
	metadata, err := backup.Merge(raw, *site, articles)
	if err != nil {
		return "", err
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
	for key, value := range newer {
		old[key] = value
	}
	merged, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(newPath, merged, 0600)
}
