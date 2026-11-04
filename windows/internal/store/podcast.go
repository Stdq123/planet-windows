package store

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"windows/internal/siteimage"
)

var podcastLanguagePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)
var podcastCategories = map[string]bool{"Arts": true, "Business": true, "Comedy": true, "Education": true, "Fiction": true, "Government": true, "History": true, "Health & Fitness": true, "Kids & Family": true, "Leisure": true, "Music": true, "News": true, "Religion & Spirituality": true, "Science": true, "Society & Culture": true, "Sports": true, "Technology": true, "True Crime": true, "TV & Film": true}

func (s *Store) UpdatePodcastSettings(siteID, language string, explicit bool, categories []string) (Site, error) {
	language = strings.TrimSpace(language)
	if language == "" {
		language = "en"
	}
	if !podcastLanguagePattern.MatchString(language) {
		return Site{}, errors.New("播客语言无效，请使用例如 en 或 zh-CN")
	}
	seen := map[string]bool{}
	for _, category := range categories {
		if !podcastCategories[category] || seen[category] {
			return Site{}, errors.New("播客分类无效或重复")
		}
		seen[category] = true
	}
	categories = append([]string{}, categories...)
	sort.Strings(categories)
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		if site.Imported {
			return Site{}, errors.New("导入站点暂不支持修改播客设置")
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.PodcastLanguage, site.PodcastExplicit, site.PodcastCategories = language, explicit, categories
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}

func (s *Store) SetPodcastCover(siteID, value string) (Site, error) {
	if value != "" {
		if _, _, err := siteimage.DecodePNG(value); err != nil {
			return Site{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		if site.Imported {
			return Site{}, errors.New("导入站点暂不支持修改播客封面")
		}
		if site.PodcastCoverPNG == value {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.PodcastCoverPNG = value
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
