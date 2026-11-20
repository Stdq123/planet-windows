package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FolderPublishRecord struct {
	CID         string    `json:"cid"`
	PublishedAt time.Time `json:"publishedAt"`
}

type PublishedFolder struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Path          string                `json:"path"`
	KeyName       string                `json:"keyName"`
	IPNS          string                `json:"ipns,omitempty"`
	LastCID       string                `json:"lastCid,omitempty"`
	LastPublished *time.Time            `json:"lastPublished,omitempty"`
	AutoPublish   bool                  `json:"autoPublish,omitempty"`
	History       []FolderPublishRecord `json:"history,omitempty"`
	Created       time.Time             `json:"created"`
}

func cloneFolder(f PublishedFolder) PublishedFolder {
	f.History = append([]FolderPublishRecord(nil), f.History...)
	if f.LastPublished != nil {
		t := *f.LastPublished
		f.LastPublished = &t
	}
	return f
}

func (s *Store) ListPublishedFolders() ([]PublishedFolder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PublishedFolder, 0, len(s.data.PublishedFolders))
	for _, f := range s.data.PublishedFolders {
		out = append(out, cloneFolder(f))
	}
	return out, nil
}

func (s *Store) AddPublishedFolder(name, dirPath string, autoPublish bool) (PublishedFolder, error) {
	dirPath = strings.TrimSpace(dirPath)
	if dirPath == "" {
		return PublishedFolder{}, errors.New("请选择要发布的本地文件夹")
	}
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		return PublishedFolder{}, errors.New("目标路径不是有效的本地文件夹")
	}
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return PublishedFolder{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(absPath)
	}
	id, err := newID()
	if err != nil {
		return PublishedFolder{}, err
	}
	folder := PublishedFolder{
		ID:          id,
		Name:        name,
		Path:        absPath,
		KeyName:     "folder-" + id,
		AutoPublish: autoPublish,
		Created:     time.Now().UTC(),
		History:     []FolderPublishRecord{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.PublishedFolders = append(append([]PublishedFolder(nil), s.data.PublishedFolders...), folder)
	if err := s.write(next); err != nil {
		return PublishedFolder{}, err
	}
	return cloneFolder(folder), nil
}

func (s *Store) RecordFolderPublication(folderID, ipns, cid string) (PublishedFolder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.PublishedFolders = append([]PublishedFolder(nil), s.data.PublishedFolders...)
	for i := range next.PublishedFolders {
		if next.PublishedFolders[i].ID != folderID {
			continue
		}
		now := time.Now().UTC()
		next.PublishedFolders[i].IPNS = ipns
		next.PublishedFolders[i].LastCID = cid
		next.PublishedFolders[i].LastPublished = &now
		hist := append([]FolderPublishRecord{{CID: cid, PublishedAt: now}}, next.PublishedFolders[i].History...)
		if len(hist) > 50 {
			hist = hist[:50]
		}
		next.PublishedFolders[i].History = hist
		if err := s.write(next); err != nil {
			return PublishedFolder{}, err
		}
		return cloneFolder(next.PublishedFolders[i]), nil
	}
	return PublishedFolder{}, errors.New("发布文件夹不存在")
}

func (s *Store) RemovePublishedFolder(folderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.PublishedFolders = make([]PublishedFolder, 0, len(s.data.PublishedFolders))
	found := false
	for _, f := range s.data.PublishedFolders {
		if f.ID == folderID {
			found = true
			continue
		}
		next.PublishedFolders = append(next.PublishedFolders, f)
	}
	if !found {
		return errors.New("发布文件夹不存在")
	}
	return s.write(next)
}

func (s *Store) UpdateSiteAggregation(siteID string, sources []string) (Site, error) {
	cleaned := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, raw := range sources {
		u := strings.TrimSpace(raw)
		if u == "" || seen[strings.ToLower(u)] {
			continue
		}
		seen[strings.ToLower(u)] = true
		cleaned = append(cleaned, u)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		site.Aggregation = cleaned
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
