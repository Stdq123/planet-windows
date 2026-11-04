// Package attachments manages files belonging to Windows-authored articles.
package attachments

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"windows/internal/backup"
	"windows/internal/mediainfo"
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
	return managedDir(root, "attachments", siteID, articleID, create)
}

func managedDir(root, kind, siteID, articleID string, create bool) (string, error) {
	if !backup.SafeName(siteID) || !backup.SafeName(articleID) {
		return "", errors.New("附件路径无效")
	}
	dir := root
	for _, part := range []string{kind, siteID, articleID} {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(dir, 0700); err != nil {
				return "", err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("附件目录无效")
		}
	}
	return dir, nil
}

// AddDraft keeps newly selected files outside all public and backup trees.
func AddDraft(root string, library *store.Store, siteID, draftID, source string) (store.Draft, error) {
	name := filepath.Base(source)
	if !validName(name) || strings.HasPrefix(strings.ToLower(name), ".draft-stage-") {
		return store.Draft{}, errors.New("附件文件名无效或与生成文件冲突")
	}
	CleanupDraftFiles(root, library)
	for _, entry := range library.PendingDraftCleanup() {
		if entry.SiteID == siteID && entry.DraftID == draftID && strings.EqualFold(entry.Name, name) {
			return store.Draft{}, errors.New("原草稿附件等待清理，请稍后重试")
		}
	}
	drafts, err := library.ListDrafts(siteID)
	if err != nil {
		return store.Draft{}, err
	}
	found := false
	for _, draft := range drafts {
		if draft.ID == draftID {
			found = true
			break
		}
	}
	if !found {
		return store.Draft{}, errors.New("草稿不存在")
	}
	dir, err := managedDir(root, "draft-attachments", siteID, draftID, true)
	if err != nil {
		return store.Draft{}, err
	}
	target := filepath.Join(dir, name)
	_, err = os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		if err = library.BeginDraftAttachment(siteID, draftID, name); err != nil {
			return store.Draft{}, err
		}
	} else if err != nil {
		return store.Draft{}, err
	}
	_, err = copyOrReuse(source, target, dir)
	if err != nil {
		CleanupDraftFiles(root, library)
		return store.Draft{}, err
	}
	draft, err := library.ChangeDraftAttachment(siteID, draftID, name, true)
	if err != nil {
		CleanupDraftFiles(root, library)
	}
	return draft, err
}

// RemoveDraft removes a newly added draft file without changing its article.
func RemoveDraft(root string, library *store.Store, siteID, draftID, name string) (store.Draft, error) {
	if !validName(name) {
		return store.Draft{}, errors.New("附件名称无效")
	}
	drafts, err := library.ListDrafts(siteID)
	if err != nil {
		return store.Draft{}, err
	}
	found := false
	for _, draft := range drafts {
		if draft.ID == draftID {
			for _, item := range draft.Attachments {
				if item == name {
					found = true
				}
			}
		}
	}
	if !found {
		return store.Draft{}, errors.New("草稿附件不存在")
	}
	draft, err := library.ChangeDraftAttachment(siteID, draftID, name, false)
	if err != nil {
		return store.Draft{}, err
	}
	CleanupDraftFiles(root, library)
	return draft, nil
}

// PublishDraft stages files before committing article and draft state together.
func PublishDraft(root string, library *store.Store, siteID, draftID, title, content string) (store.Article, error) {
	return publishDraft(root, library, siteID, draftID, title, content, nil)
}

func PublishDraftWithTags(root string, library *store.Store, siteID, draftID, title, content, tagsJSON string) (store.Article, error) {
	return publishDraft(root, library, siteID, draftID, title, content, &tagsJSON)
}

func publishDraft(root string, library *store.Store, siteID, draftID, title, content string, tagsJSON *string) (store.Article, error) {
	CleanupArticleFiles(root, library)
	prepare := func(article store.Article, names []string) (store.Article, func(), error) {
		src, err := managedDir(root, "draft-attachments", siteID, draftID, false)
		if err != nil {
			return store.Article{}, nil, err
		}
		dst, err := ownedDir(root, siteID, article.ID, true)
		if err != nil {
			return store.Article{}, nil, err
		}
		var created []string
		rollback := func() {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
		article.Attachments = append([]string(nil), article.Attachments...)
		for _, name := range names {
			if !validName(name) {
				rollback()
				return store.Article{}, nil, errors.New("草稿附件名称无效")
			}
			for _, old := range article.Attachments {
				if strings.EqualFold(old, name) {
					rollback()
					return store.Article{}, nil, errors.New("文章已有同名附件")
				}
			}
			target := filepath.Join(dst, name)
			wasCreated, err := copyOrReuse(filepath.Join(src, name), target, src)
			if err != nil {
				rollback()
				return store.Article{}, nil, err
			}
			if wasCreated {
				created = append(created, target)
			}
			article.Attachments = append(article.Attachments, name)
			switch strings.ToLower(filepath.Ext(name)) {
			case ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".opus", ".flac":
				info, err := os.Stat(target)
				if err != nil {
					rollback()
					return store.Article{}, nil, err
				}
				size := info.Size()
				article.AudioFilename, article.AudioByteLength, article.AudioDuration = name, &size, mediainfo.Duration(target)
			case ".mp4", ".mov", ".m4v", ".webm", ".avi", ".mpeg", ".mpg":
				article.VideoFilename = name
			}
		}
		return article, rollback, nil
	}
	var article store.Article
	var err error
	if tagsJSON != nil {
		article, err = library.PublishDraftPreparedWithTags(siteID, draftID, title, content, *tagsJSON, prepare)
	} else {
		article, err = library.PublishDraftPrepared(siteID, draftID, title, content, prepare)
	}
	if err == nil {
		CleanupDraftFiles(root, library)
		CleanupArticleFiles(root, library)
	}
	return article, err
}

// CleanupArticleFiles removes only locally owned files queued by a committed
// draft publication; files still referenced by any article remain untouched.
func CleanupArticleFiles(root string, library *store.Store) {
	protected := map[string]bool{}
	for _, entry := range library.ArticleFileReferences() {
		protected[strings.ToLower(filepath.Join(entry.SiteID, entry.ArticleID, entry.Name))] = true
	}
	for _, entry := range library.PendingArticleCleanup() {
		if !validName(entry.Name) {
			continue
		}
		if protected[strings.ToLower(filepath.Join(entry.SiteID, entry.ArticleID, entry.Name))] {
			_ = library.CompleteArticleCleanup(entry)
			continue
		}
		dir, err := ownedDir(root, entry.SiteID, entry.ArticleID, false)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteArticleCleanup(entry)
			continue
		}
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteArticleCleanup(entry)
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteArticleCleanup(entry)
		}
	}
}

// CleanupDraftFiles retries only removals recorded in the durable store.
func CleanupDraftFiles(root string, library *store.Store) {
	protected := map[string]bool{}
	for _, entry := range library.DraftFileReferences() {
		protected[strings.ToLower(filepath.Join(entry.SiteID, entry.DraftID, entry.Name))] = true
	}
	for _, entry := range library.PendingDraftStaging() {
		if !validName(entry.Name) {
			continue
		}
		if protected[strings.ToLower(filepath.Join(entry.SiteID, entry.DraftID, entry.Name))] {
			_ = library.CompleteDraftStaging(entry)
			continue
		}
		dir, err := managedDir(root, "draft-attachments", entry.SiteID, entry.DraftID, false)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftStaging(entry)
			continue
		}
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftStaging(entry)
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftStaging(entry)
		}
	}
	for _, entry := range library.PendingDraftCleanup() {
		if !validName(entry.Name) {
			continue
		}
		dir, err := managedDir(root, "draft-attachments", entry.SiteID, entry.DraftID, false)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftCleanup(entry)
			continue
		}
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftCleanup(entry)
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err = os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			_ = library.CompleteDraftCleanup(entry)
		}
	}
	// Interrupted copies can leave private staging files before registration.
	// Never remove a name recorded by any draft, including a removed site's.
	base := filepath.Join(root, "draft-attachments")
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	sites, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, site := range sites {
		if !backup.SafeName(site.Name()) {
			continue
		}
		sitePath := filepath.Join(base, site.Name())
		info, err := os.Lstat(sitePath)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		drafts, err := os.ReadDir(sitePath)
		if err != nil {
			continue
		}
		for _, draft := range drafts {
			if !backup.SafeName(draft.Name()) {
				continue
			}
			dir, err := managedDir(root, "draft-attachments", site.Name(), draft.Name(), false)
			if err != nil {
				continue
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, file := range files {
				name := file.Name()
				if !strings.HasPrefix(strings.ToLower(name), ".draft-stage-") || protected[strings.ToLower(filepath.Join(site.Name(), draft.Name(), name))] {
					continue
				}
				path := filepath.Join(dir, name)
				info, err := os.Lstat(path)
				if err == nil && info.Mode().IsRegular() {
					_ = os.Remove(path)
				}
			}
		}
	}
}

// copyOrReuse uses a completed temporary copy and a no-replace hard link. An
// identical leftover from an interrupted commit can safely be reused.
func copyOrReuse(src, dst, temporaryDir string) (bool, error) {
	if _, err := os.Lstat(dst); err == nil {
		equal, err := sameRegular(src, dst)
		if err != nil {
			return false, err
		}
		if !equal {
			return false, errors.New("存在不同内容的同名残留附件")
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	temp, err := os.CreateTemp(temporaryDir, ".draft-stage-*")
	if err != nil {
		return false, err
	}
	name := temp.Name()
	if err = temp.Close(); err != nil {
		_ = os.Remove(name)
		return false, err
	}
	if err = os.Remove(name); err != nil {
		return false, err
	}
	defer os.Remove(name)
	if err = copyRegular(src, name); err != nil {
		return false, err
	}
	if err = os.Link(name, dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			equal, e := sameRegular(src, dst)
			if e == nil && equal {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func sameRegular(a, b string) (bool, error) {
	digest := func(path string) ([32]byte, error) {
		var zero [32]byte
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return zero, errors.New("附件必须是普通文件")
		}
		f, err := os.Open(path)
		if err != nil {
			return zero, err
		}
		defer f.Close()
		actual, err := f.Stat()
		if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
			return zero, errors.New("附件在读取期间发生变化")
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return zero, err
		}
		var sum [32]byte
		copy(sum[:], h.Sum(nil))
		return sum, nil
	}
	x, err := digest(a)
	if err != nil {
		return false, err
	}
	y, err := digest(b)
	if err != nil {
		return false, err
	}
	return x == y, nil
}

func ReadDraftPreviewImage(root string, draft store.Draft, name string) ([]byte, string, error) {
	if !validName(name) {
		return nil, "", errors.New("附件名称无效")
	}
	found := false
	for _, item := range draft.Attachments {
		if item == name {
			found = true
			break
		}
	}
	if !found {
		return nil, "", errors.New("草稿附件未登记")
	}
	dir, err := managedDir(root, "draft-attachments", draft.SiteID, draft.ID, false)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", errors.New("草稿附件文件无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, "", errors.New("草稿附件在读取期间发生变化")
	}
	return readPreviewImage(f)
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
	CleanupArticleFiles(root, library)
	for _, entry := range library.PendingArticleCleanup() {
		if entry.SiteID == siteID && entry.ArticleID == articleID && strings.EqualFold(entry.Name, name) {
			return store.Article{}, errors.New("原附件等待清理，请稍后重试")
		}
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
	info, err := os.Stat(target)
	if err != nil {
		_ = os.Remove(target)
		return store.Article{}, err
	}
	size := info.Size()
	var duration *int
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".opus", ".flac":
		duration = mediainfo.Duration(target)
	}
	article, err := library.AddAttachmentWithMetadata(siteID, articleID, name, duration, &size)
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
		if !errors.Is(err, os.ErrNotExist) {
			return store.Article{}, err
		}
		// Original imported files remain in the immutable archive.
		sites, e := library.ListSites()
		if e != nil {
			return store.Article{}, e
		}
		for _, site := range sites {
			if site.ID == siteID && site.Imported {
				info, e := os.Lstat(filepath.Join(root, "imports", siteID, articleID, name))
				if e != nil || !info.Mode().IsRegular() {
					return store.Article{}, errors.New("附件文件无效")
				}
				return library.RemoveAttachment(siteID, articleID, name)
			}
		}
		return store.Article{}, err
	}
	path := filepath.Join(dir, name)
	if _, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		info, e := os.Lstat(filepath.Join(root, "imports", siteID, articleID, name))
		if e != nil || !info.Mode().IsRegular() {
			return store.Article{}, errors.New("附件文件无效")
		}
		return library.RemoveAttachment(siteID, articleID, name)
	}
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

// CopyImportedCurrent combines owned edits with unchanged archived attachments.
func CopyImportedCurrent(root, archive, destination string, article store.Article) error {
	if !backup.SafeName(article.SiteID) || !backup.SafeName(article.ID) {
		return errors.New("附件路径无效")
	}
	seen := map[string]bool{}
	for _, name := range article.Attachments {
		key := strings.ToLower(name)
		if !validName(name) || seen[key] {
			return errors.New("附件名称无效或冲突")
		}
		seen[key] = true
		source := ""
		dir, err := ownedDir(root, article.SiteID, article.ID, false)
		if err == nil {
			candidate := filepath.Join(dir, name)
			if _, e := os.Lstat(candidate); e == nil {
				source = candidate
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if source == "" {
			dir = filepath.Join(archive, article.ID)
			info, e := os.Lstat(dir)
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("原版文章目录无效")
			}
			source = filepath.Join(dir, name)
		}
		if err := copyRegular(source, filepath.Join(destination, article.ID, name)); err != nil {
			return fmt.Errorf("复制附件 %s 失败：%w", name, err)
		}
	}
	return nil
}

// OpenRegistered opens a registered attachment without following links.
func OpenRegistered(root string, site store.Site, article store.Article, name string) (*os.File, error) {
	if site.ID != article.SiteID || !backup.SafeName(site.ID) || !backup.SafeName(article.ID) || !validName(name) {
		return nil, errors.New("附件路径无效")
	}
	registered := false
	for _, item := range article.Attachments {
		if item == name {
			registered = true
			break
		}
	}
	if !registered {
		return nil, errors.New("附件未登记")
	}
	var path string
	dir, err := ownedDir(root, site.ID, article.ID, false)
	if err == nil {
		candidate := filepath.Join(dir, name)
		if _, err = os.Lstat(candidate); err == nil {
			path = candidate
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if path == "" {
		if !site.Imported {
			return nil, os.ErrNotExist
		}
		dir = root
		for _, part := range []string{"imports", site.ID, article.ID} {
			dir = filepath.Join(dir, part)
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("原附件目录无效")
			}
		}
		path = filepath.Join(dir, name)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("附件文件无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		f.Close()
		return nil, errors.New("附件在读取期间发生变化")
	}
	return f, nil
}

// ReadPreviewImage reads only a registered raster attachment from the current article.
func ReadPreviewImage(root string, site store.Site, article store.Article, name string) ([]byte, string, error) {
	f, err := OpenRegistered(root, site, article, name)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return readPreviewImage(f)
}

func readPreviewImage(f *os.File) ([]byte, string, error) {
	const limit = 8 << 20
	info, err := f.Stat()
	if err != nil || info.Size() > limit {
		return nil, "", errors.New("图片无效或过大")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(data) > limit {
		return nil, "", errors.New("图片读取失败或过大")
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return nil, "", errors.New("不支持预览此图片格式")
	}
	return data, mime, nil
}

// EnrichMedia derives optional audio metadata from the current registered file.
func EnrichMedia(root string, site store.Site, articles []store.Article) ([]store.Article, error) {
	out := append([]store.Article(nil), articles...)
	for i := range out {
		article := &out[i]
		if article.AudioFilename == "" {
			continue
		}
		f, err := OpenRegistered(root, site, *article, article.AudioFilename)
		if err != nil {
			return nil, err
		}
		info, err := f.Stat()
		path := f.Name()
		f.Close()
		if err != nil {
			return nil, err
		}
		size := info.Size()
		article.AudioByteLength = &size
		if article.AudioDuration == nil {
			article.AudioDuration = mediainfo.Duration(path)
		}
	}
	return out, nil
}

// GenerateVideoThumbnail creates a public poster if the system can extract one.
func GenerateVideoThumbnail(root string, site store.Site, article store.Article, destination string) error {
	if article.VideoFilename == "" {
		return nil
	}
	target := filepath.Join(destination, article.ID, "_videoThumbnail.png")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("视频缩略图无效")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := OpenRegistered(root, site, article, article.VideoFilename)
	if err != nil {
		return err
	}
	path := file.Name()
	file.Close()
	data, err := mediainfo.VideoThumbnail(path)
	if err != nil {
		return nil
	}
	return os.WriteFile(target, data, 0600)
}
