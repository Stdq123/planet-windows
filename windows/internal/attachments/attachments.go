// Package attachments manages files belonging to Windows-authored articles.
package attachments

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"windows/internal/backup"
	"windows/internal/store"
)

func validName(name string) bool {
	if !backup.SafeName(name) || strings.HasSuffix(strings.ToLower(name), ".key") {
		return false
	}
	switch strings.ToLower(name) {
	case "index.html", "article.json", "article.md", "simple.html", "nft.json", "nft.json.cid.txt", "_cover.png", "_grid.jpg", "_videothumbnail.png":
		return false
	}
	return true
}

// CopyImported copies only the attachment names recorded in the original backup.
func CopyImported(sourceDir, destinationDir string, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		folded := strings.ToLower(name)
		if !validName(name) || seen[folded] {
			return errors.New("原版附件名称无效或冲突")
		}
		seen[folded] = true
		if err := copyRegular(filepath.Join(sourceDir, name), filepath.Join(destinationDir, name)); err != nil {
			return fmt.Errorf("复制原版附件 %s 失败：%w", name, err)
		}
	}
	return nil
}

func ownedDir(root, siteID, articleID string, create bool) (string, error) {
	if !backup.SafeName(siteID) || !backup.SafeName(articleID) {
		return "", errors.New("附件路径无效")
	}
	dir := root
	for _, part := range []string{"attachments", siteID, articleID} {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(dir, 0700); err != nil {
				return "", err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("附件目录无效")
		}
	}
	return dir, nil
}
func copyRegular(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("附件必须是普通文件")
	}
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	actual, err := input.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return errors.New("附件在复制期间发生变化")
	}
	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

// Add copies a file and records it on a saved, Windows-authored article.
func Add(root string, library *store.Store, siteID, articleID, source string) (store.Article, error) {
	name := filepath.Base(source)
	if !validName(name) {
		return store.Article{}, errors.New("附件文件名无效或与生成文件冲突")
	}
	posts, err := library.ListArticles(siteID)
	if err != nil {
		return store.Article{}, err
	}
	found := false
	for _, post := range posts {
		if post.ID == articleID {
			found = true
			for _, old := range post.Attachments {
				if strings.EqualFold(old, name) {
					return store.Article{}, errors.New("附件名称已存在")
				}
			}
			break
		}
	}
	if !found {
		return store.Article{}, errors.New("文章不存在")
	}
	dir, err := ownedDir(root, siteID, articleID, true)
	if err != nil {
		return store.Article{}, err
	}
	target := filepath.Join(dir, name)
	if err = copyRegular(source, target); err != nil {
		return store.Article{}, err
	}
	article, err := library.AddAttachment(siteID, articleID, name)
	if err != nil {
		_ = os.Remove(target)
		return store.Article{}, err
	}
	return article, nil
}

// Remove removes only an attachment registered to this article.
func Remove(root string, library *store.Store, siteID, articleID, name string) (store.Article, error) {
	if !validName(name) {
		return store.Article{}, errors.New("附件名称无效")
	}
	posts, err := library.ListArticles(siteID)
	if err != nil {
		return store.Article{}, err
	}
	found := false
	for _, post := range posts {
		if post.ID == articleID {
			for _, old := range post.Attachments {
				if old == name {
					found = true
				}
			}
			break
		}
	}
	if !found {
		return store.Article{}, errors.New("附件不存在")
	}
	dir, err := ownedDir(root, siteID, articleID, false)
	if err != nil {
		return store.Article{}, err
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return store.Article{}, errors.New("附件文件无效")
	}
	temp, err := os.CreateTemp(dir, ".remove-*")
	if err != nil {
		return store.Article{}, err
	}
	tempPath := temp.Name()
	if err = temp.Close(); err != nil {
		return store.Article{}, err
	}
	if err = os.Remove(tempPath); err != nil {
		return store.Article{}, err
	}
	if err = os.Rename(path, tempPath); err != nil {
		return store.Article{}, err
	}
	article, err := library.RemoveAttachment(siteID, articleID, name)
	if err != nil {
		if rollbackErr := os.Rename(tempPath, path); rollbackErr != nil {
			return store.Article{}, fmt.Errorf("%v；附件恢复失败：%w", err, rollbackErr)
		}
		return store.Article{}, err
	}
	_ = os.Remove(tempPath)
	return article, nil
}

// CopyToSite adds registered files to an already rendered public site.
func CopyToSite(root, destination string, articles []store.Article) error {
	for _, article := range articles {
		if len(article.Attachments) == 0 {
			continue
		}
		dir, err := ownedDir(root, article.SiteID, article.ID, false)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, name := range article.Attachments {
			lower := strings.ToLower(name)
			if !validName(name) || seen[lower] {
				return errors.New("附件名称无效或冲突")
			}
			seen[lower] = true
			target := filepath.Join(destination, article.ID, name)
			if err = copyRegular(filepath.Join(dir, name), target); err != nil {
				return fmt.Errorf("复制附件 %s 失败：%w", name, err)
			}
		}
	}
	return nil
}
