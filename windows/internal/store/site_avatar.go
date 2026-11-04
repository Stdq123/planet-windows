package store

import (
	"errors"
	"windows/internal/siteimage"
)

// SetSiteAvatar atomically saves a validated local site's avatar and marks it unpublished.
func (s *Store) SetSiteAvatar(siteID, avatar string) (Site, error) {
	if avatar != "" {
		if _, _, err := siteimage.DecodePNG(avatar); err != nil {
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
			return Site{}, errors.New("导入站点暂不支持修改头像")
		}
		if site.AvatarPNG == avatar {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.AvatarPNG = avatar
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
