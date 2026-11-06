package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// XPostRecord is local-only: no credentials or server response bodies belong here.
// Pending includes requests whose outcome is unknown; only explicit user action
// may retry them. Sent records survive disconnecting or replacing credentials.
type XPostRecord struct {
	Status string `json:"status"`
	PostID string `json:"postId"`
	URL    string `json:"url"`
	Error  string `json:"error,omitempty"`
}

func xPostRecordID(siteID, articleID string) (string, error) {
	if !safeID.MatchString(siteID) || !safeID.MatchString(articleID) {
		return "", errors.New("X 同步记录标识无效")
	}
	digest := sha256.Sum256([]byte(siteID + "\x00" + articleID))
	return hex.EncodeToString(digest[:]), nil
}

func validXPostRecord(record XPostRecord) bool {
	if !validXErrorMessage(record.Error) {
		return false
	}
	switch record.Status {
	case "":
		return record.PostID == "" && record.URL == "" && record.Error == ""
	case "pending", "failed":
		return record.PostID == "" && record.URL == ""
	case "sent":
		return validXPostID(record.PostID) && record.URL == "https://x.com/i/web/status/"+record.PostID && record.Error == ""
	default:
		return false
	}
}

func validXPostConfig(id string, cfg Config) bool {
	wantID, err := xPostRecordID(cfg.Name, cfg.Server)
	if err != nil || wantID != id || cfg.Token == "" {
		return false
	}
	record := XPostRecord{Status: cfg.Token, PostID: cfg.RequestID, URL: cfg.LastURL, Error: cfg.XError}
	// Reject unrelated fields, especially credentials, in the per-article store.
	want := Config{Name: cfg.Name, Server: cfg.Server, Token: cfg.Token, RequestID: cfg.RequestID, LastURL: cfg.LastURL, XError: cfg.XError}
	return cfg == want && validXPostRecord(record)
}

func SaveXPost(root, siteID, articleID string, record XPostRecord) error {
	id, err := xPostRecordID(siteID, articleID)
	if err != nil {
		return err
	}
	if !validXPostRecord(record) {
		return errors.New("X 同步记录无效")
	}
	if record.Status == "" {
		return SaveConfig(root, id, "x-post", Config{})
	}
	return SaveConfig(root, id, "x-post", Config{Name: siteID, Server: articleID, Token: record.Status, RequestID: record.PostID, LastURL: record.URL, XError: record.Error})
}

func LoadXPost(root, siteID, articleID string) (XPostRecord, error) {
	id, err := xPostRecordID(siteID, articleID)
	if err != nil {
		return XPostRecord{}, err
	}
	cfg, err := LoadConfig(root, id, "x-post")
	if err != nil || cfg.Token == "" {
		return XPostRecord{}, err
	}
	return XPostRecord{Status: cfg.Token, PostID: cfg.RequestID, URL: cfg.LastURL, Error: cfg.XError}, nil
}

// DeleteXPostsForSite validates the private directory and each record before
// removing anything. It never follows redirected paths or deletes another site.
func DeleteXPostsForSite(root, siteID string) error {
	if !safeID.MatchString(siteID) {
		return errors.New("X 同步记录标识无效")
	}
	probe, err := configPath(root, siteID, "x-post", false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(probe))
	if err != nil {
		return err
	}
	var ids []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".dat") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".dat")
		cfg, err := LoadConfig(root, id, "x-post")
		if err != nil {
			return err
		}
		if cfg.Name == siteID {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if err := SaveConfig(root, id, "x-post", Config{}); err != nil {
			return err
		}
	}
	return nil
}
