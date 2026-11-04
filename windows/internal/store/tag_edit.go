package store

import (
	"encoding/json"
	"errors"
	"strings"

	"windows/internal/tagname"
)

func parseEditedTags(raw string) (map[string]string, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("标签数据过大")
	}
	var tags map[string]string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil || tags == nil || len(tags) > 256 {
		return nil, errors.New("标签数据无效")
	}
	for key, label := range tags {
		if !tagname.Valid(key, label) {
			return nil, errors.New("标签名称或路径无效")
		}
	}
	return tags, nil
}

// validateTagsLocked includes historical imported keys because a generated tag
// path must not alias an old archive path after a later backup rebuild.
func (s *Store) validateTagsLocked(siteID, articleID string, edited map[string]string) error {
	for _, site := range s.data.Sites {
		if site.ID == siteID && site.TagsUnsupported {
			return errors.New("原备份标签格式不受支持，无法编辑标签")
		}
	}
	seen := map[string]string{}
	validate := func(tags map[string]string) error {
		for key, label := range tags {
			if !tagname.Valid(key, label) {
				return errors.New("标签名称或路径无效")
			}
			folded := tagname.FoldKey(key)
			if old, ok := seen[folded]; ok && old != key {
				return errors.New("标签路径存在大小写冲突")
			}
			seen[folded] = key
		}
		return nil
	}
	if raw := s.data.BackupMetadata[siteID]; len(raw) > 0 {
		var old struct {
			Tags     map[string]string `json:"tags"`
			Articles []struct {
				Tags map[string]string `json:"tags"`
			} `json:"articles"`
		}
		if err := json.Unmarshal(raw, &old); err != nil {
			return errors.New("原备份标签无效")
		}
		if err := validate(old.Tags); err != nil {
			return err
		}
		for _, a := range old.Articles {
			if err := validate(a.Tags); err != nil {
				return err
			}
		}
	}
	for _, a := range s.data.Articles {
		if a.SiteID == siteID && !strings.EqualFold(a.ID, articleID) {
			if err := validate(a.Tags); err != nil {
				return err
			}
		}
	}
	return validate(edited)
}

func (s *Store) SaveArticleWithTags(siteID, articleID, title, content, tagsJSON string) (Article, error) {
	tags, err := parseEditedTags(tagsJSON)
	if err != nil {
		return Article{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.validateTagsLocked(siteID, articleID, tags); err != nil {
		return Article{}, err
	}
	return s.saveArticleLocked(siteID, articleID, title, content, "", nil, tags)
}
