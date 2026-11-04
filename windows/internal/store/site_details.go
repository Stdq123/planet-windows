package store

import (
	"errors"
	"regexp"
	"strings"
)

var siteSlugPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,120}$`)

// UpdateSiteDetails saves local site identity and indexing settings together.
func (s *Store) UpdateSiteDetails(siteID, name, about string, doNotIndex bool, slug, author string) (Site, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	author = strings.TrimSpace(author)
	if name == "" {
		return Site{}, errors.New("请输入站点名称")
	}
	if slug != "" && !siteSlugPattern.MatchString(slug) {
		return Site{}, errors.New("站点标识只能包含字母、数字和连字符")
	}
	if len([]rune(author)) > 250 {
		return Site{}, errors.New("作者名称不能超过 250 个字符")
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
			return Site{}, errors.New("导入站点暂不支持修改站点标识或作者")
		}
		if site.Name == name && site.About == about && site.DoNotIndex == doNotIndex && site.Slug == slug && site.AuthorName == author {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.Name, site.About, site.DoNotIndex, site.Slug, site.AuthorName = name, about, doNotIndex, slug, author
		site.ContentRevision++
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
