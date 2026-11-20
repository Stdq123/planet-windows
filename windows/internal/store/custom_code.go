package store

import "errors"

const maxCustomCodeBytes = 64 << 10

type SiteCustomCode struct {
	HeadEnabled      bool   `json:"headEnabled"`
	Head             string `json:"head"`
	BodyStartEnabled bool   `json:"bodyStartEnabled"`
	BodyStart        string `json:"bodyStart"`
	BodyEndEnabled   bool   `json:"bodyEndEnabled"`
	BodyEnd          string `json:"bodyEnd"`
	PublishAsIPNS    *bool  `json:"publishAsIPNS,omitempty"`
	PrewarmNewPost   *bool  `json:"prewarmNewPost,omitempty"`
}

func boolPtrEqual(a, b *bool) bool {
	aVal := a == nil || *a
	bVal := b == nil || *b
	return aVal == bVal
}

// UpdateSiteCustomCode saves custom HTML snippets and publication flags for a native site.
func (s *Store) UpdateSiteCustomCode(siteID string, cfg SiteCustomCode) (Site, error) {
	if len(cfg.Head) > maxCustomCodeBytes || len(cfg.BodyStart) > maxCustomCodeBytes || len(cfg.BodyEnd) > maxCustomCodeBytes {
		return Site{}, errors.New("自定义代码单段不能超过 64 KiB")
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
			return Site{}, errors.New("导入站点暂不支持修改自定义代码或发布偏好")
		}
		if site.CustomCodeHeadEnabled == cfg.HeadEnabled &&
			site.CustomCodeHead == cfg.Head &&
			site.CustomCodeBodyStartEnabled == cfg.BodyStartEnabled &&
			site.CustomCodeBodyStart == cfg.BodyStart &&
			site.CustomCodeBodyEndEnabled == cfg.BodyEndEnabled &&
			site.CustomCodeBodyEnd == cfg.BodyEnd &&
			boolPtrEqual(site.PublishAsIPNS, cfg.PublishAsIPNS) &&
			boolPtrEqual(site.PrewarmNewPost, cfg.PrewarmNewPost) {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.CustomCodeHeadEnabled = cfg.HeadEnabled
		site.CustomCodeHead = cfg.Head
		site.CustomCodeBodyStartEnabled = cfg.BodyStartEnabled
		site.CustomCodeBodyStart = cfg.BodyStart
		site.CustomCodeBodyEndEnabled = cfg.BodyEndEnabled
		site.CustomCodeBodyEnd = cfg.BodyEnd
		if cfg.PublishAsIPNS != nil {
			v := *cfg.PublishAsIPNS
			site.PublishAsIPNS = &v
		}
		if cfg.PrewarmNewPost != nil {
			v := *cfg.PrewarmNewPost
			site.PrewarmNewPost = &v
		}
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
