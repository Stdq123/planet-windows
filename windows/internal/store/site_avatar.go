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
		if avatar != "" && site.SaveRoundAvatar {
			cropped, err := siteimage.CircleCropPNG(avatar)
			if err != nil {
				return Site{}, err
			}
			avatar = cropped
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

// SetSaveRoundAvatar updates the site's round avatar preference and crops the existing avatar if enabled.
func (s *Store) SetSaveRoundAvatar(siteID string, round bool) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		nextAvatar := site.AvatarPNG
		if round && nextAvatar != "" {
			cropped, err := siteimage.CircleCropPNG(nextAvatar)
			if err != nil {
				return Site{}, err
			}
			nextAvatar = cropped
		}
		if site.SaveRoundAvatar == round && site.AvatarPNG == nextAvatar {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.SaveRoundAvatar = round
		site.AvatarPNG = nextAvatar
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
