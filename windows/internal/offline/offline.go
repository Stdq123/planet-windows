// Package offline stores bounded, immutable snapshots of feed media.
package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"windows/internal/feeds"
	"windows/internal/preview"
)

var snapshotName = regexp.MustCompile(`^snapshot-[A-Za-z0-9]+$`)
var fileName = regexp.MustCompile(`^[a-f0-9]{64}\.media$`)

const maxImage int64 = 8 << 20
const maxMedia int64 = 256 << 20
const maxTotal int64 = 1 << 30
const maxAssets = 256

type candidate struct{ url, kind string }

func canonical(base, raw string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u = b.ResolveReference(u)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("媒体地址无效")
	}
	u.Fragment = ""
	return u.String(), nil
}
func baseFor(source string, item feeds.Item) string {
	if u, err := url.Parse(item.Link); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil {
		return u.String()
	}
	return source
}
func candidates(source string, item feeds.Item) ([]candidate, error) {
	base := baseFor(source, item)
	list := []candidate{}
	add := func(raw, kind string) {
		if raw == "" {
			return
		}
		u, err := canonical(base, raw)
		if err == nil {
			list = append(list, candidate{u, kind})
		}
	}
	for _, m := range item.Media {
		switch {
		case strings.HasPrefix(m.Type, "image/"):
			add(m.URL, "image")
		case strings.HasPrefix(m.Type, "audio/"):
			add(m.URL, "audio")
		case strings.HasPrefix(m.Type, "video/"):
			add(m.URL, "video")
		}
	}
	add(item.AudioURL, "audio")
	add(item.VideoURL, "video")
	html := item.Content
	if html == "" {
		html = item.Description
	}
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(html), context)
	if err != nil {
		return nil, err
	}
	var walk func(*xhtml.Node, string)
	walk = func(n *xhtml.Node, parent string) {
		kind := ""
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "img":
				kind = "image"
			case "audio":
				kind = "audio"
			case "video":
				kind = "video"
			case "source":
				if parent == "audio" || parent == "video" {
					kind = parent
				}
			}
			for _, a := range n.Attr {
				if a.Key == "srcset" && (n.Data == "img" || (n.Data == "source" && parent == "picture")) && !strings.Contains(strings.ToLower(a.Val), "data:") {
					for _, part := range strings.Split(a.Val, ",") {
						fields := strings.Fields(part)
						if len(fields) > 0 {
							add(fields[0], "image")
						}
					}
				}
				if a.Key == "src" && kind != "" {
					add(a.Val, kind)
				}
				if n.Data == "video" && a.Key == "poster" {
					add(a.Val, "image")
				}
			}
			parent = n.Data
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, parent)
		}
	}
	for _, n := range nodes {
		walk(n, "")
	}
	return list, nil
}
func publicHost(host string) (bool, error) {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return false, nil
	}
	ips := []net.IP{}
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		records, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return false, err
		}
		for _, record := range records {
			ips = append(ips, record.IP)
		}
	}
	if len(ips) == 0 {
		return false, errors.New("媒体域名无法解析")
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return false, nil
		}
	}
	return true, nil
}
func allowedURL(source, address string) error {
	origin, e := url.Parse(source)
	if e != nil {
		return e
	}
	target, e := url.Parse(address)
	if e != nil || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return errors.New("媒体地址无效")
	}
	if strings.EqualFold(origin.Hostname(), target.Hostname()) {
		return nil
	}
	public, err := publicHost(target.Hostname())
	if err != nil {
		return fmt.Errorf("媒体域名无法解析：%w", err)
	}
	if !public {
		return errors.New("媒体地址指向私有网络")
	}
	return nil
}

func mediaRoot(root string) (string, error) {
	dir := filepath.Join(root, "offline-media")
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		info, err = os.Lstat(dir)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("离线缓存目录无效")
	}
	return dir, nil
}

// Pin downloads all supported media before returning a complete snapshot.
func Pin(root, source string, feed feeds.Feed) (feeds.Feed, string, error) {
	return PinWithPrevious(root, source, feed, feeds.Feed{})
}

func PinWithPrevious(root, source string, feed, previous feeds.Feed) (feeds.Feed, string, error) {
	old := map[string]feeds.OfflineAsset{}
	for _, item := range previous.Items {
		for address, asset := range item.OfflineMedia {
			old[address] = asset
		}
	}
	parent, err := mediaRoot(root)
	if err != nil {
		return feeds.Feed{}, "", err
	}
	dir, err := os.MkdirTemp(parent, "snapshot-")
	if err != nil {
		return feeds.Feed{}, "", err
	}
	snapshot := filepath.Base(dir)
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(dir)
		}
	}()
	feed.Items = append([]feeds.Item(nil), feed.Items...)
	downloaded := map[string]feeds.OfflineAsset{}
	total := int64(0)
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("媒体重定向次数过多")
		}
		return allowedURL(source, req.URL.String())
	}}
	for i := range feed.Items {
		item := &feed.Items[i]
		item.OfflineBase = baseFor(source, *item)
		item.OfflineMedia = map[string]feeds.OfflineAsset{}
		list, err := candidates(source, *item)
		if err != nil {
			return feeds.Feed{}, "", err
		}
		for _, entry := range list {
			if asset, ok := downloaded[entry.url]; ok {
				item.OfflineMedia[entry.url] = asset
				continue
			}
			if len(downloaded) >= maxAssets {
				return feeds.Feed{}, "", errors.New("离线媒体数量超过限制")
			}
			if err = allowedURL(source, entry.url); err != nil {
				return feeds.Feed{}, "", err
			}
			req, err := http.NewRequest("GET", entry.url, nil)
			if err != nil {
				return feeds.Feed{}, "", err
			}
			req.Header.Set("User-Agent", "Planet-Windows/0.1")
			previousAsset, hasPrevious := old[entry.url]
			if hasPrevious && strings.HasPrefix(previousAsset.ContentType, entry.kind+"/") {
				f, e := Open(root, previousAsset)
				if e != nil {
					hasPrevious = false
				} else {
					f.Close()
				}
			} else {
				hasPrevious = false
			}
			if hasPrevious && previousAsset.ETag != "" {
				req.Header.Set("If-None-Match", previousAsset.ETag)
			} else if hasPrevious && previousAsset.LastModified != "" {
				req.Header.Set("If-Modified-Since", previousAsset.LastModified)
			}
			response, err := client.Do(req)
			if err != nil {
				return feeds.Feed{}, "", fmt.Errorf("下载媒体失败：%w", err)
			}
			if response.StatusCode == http.StatusNotModified && hasPrevious {
				response.Body.Close()
				total += previousAsset.Size
				if total > maxTotal {
					return feeds.Feed{}, "", errors.New("离线媒体超过总大小限制")
				}
				downloaded[entry.url] = previousAsset
				item.OfflineMedia[entry.url] = previousAsset
				if entry.kind == "audio" && item.AudioURL == "" {
					item.AudioURL = entry.url
				}
				if entry.kind == "video" && item.VideoURL == "" {
					item.VideoURL = entry.url
				}
				continue
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				response.Body.Close()
				return feeds.Feed{}, "", fmt.Errorf("下载媒体失败：HTTP %d", response.StatusCode)
			}
			limit := maxMedia
			if entry.kind == "image" {
				limit = maxImage
			}
			if response.ContentLength > limit {
				response.Body.Close()
				return feeds.Feed{}, "", errors.New("媒体文件超过大小限制")
			}
			hash := sha256.Sum256([]byte(entry.url))
			file := hex.EncodeToString(hash[:]) + ".media"
			path := filepath.Join(dir, file)
			output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				response.Body.Close()
				return feeds.Feed{}, "", err
			}
			contentHash := sha256.New()
			size, copyErr := io.Copy(io.MultiWriter(output, contentHash), io.LimitReader(response.Body, limit+1))
			response.Body.Close()
			closeErr := output.Close()
			if copyErr != nil {
				return feeds.Feed{}, "", copyErr
			}
			if closeErr != nil {
				return feeds.Feed{}, "", closeErr
			}
			if size > limit || total+size > maxTotal {
				return feeds.Feed{}, "", errors.New("离线媒体超过总大小限制")
			}
			total += size
			mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0]))
			if entry.kind == "image" {
				f, e := os.Open(path)
				if e != nil {
					return feeds.Feed{}, "", e
				}
				var head [512]byte
				n, _ := f.Read(head[:])
				f.Close()
				mediaType = http.DetectContentType(head[:n])
				if mediaType != "image/png" && mediaType != "image/jpeg" && mediaType != "image/gif" && mediaType != "image/webp" {
					return feeds.Feed{}, "", errors.New("离线图片格式不受支持")
				}
			} else if !strings.HasPrefix(mediaType, entry.kind+"/") {
				if mediaType == "" || mediaType == "application/octet-stream" {
					for _, declared := range item.Media {
						address, e := canonical(baseFor(source, *item), declared.URL)
						parsed, _, parseErr := mime.ParseMediaType(declared.Type)
						if e == nil && parseErr == nil && address == entry.url && strings.HasPrefix(parsed, entry.kind+"/") {
							mediaType = parsed
							break
						}
					}
					if u, e := url.Parse(entry.url); e == nil && (mediaType == "" || mediaType == "application/octet-stream") {
						mediaType = strings.SplitN(mime.TypeByExtension(filepath.Ext(u.Path)), ";", 2)[0]
					}
				}
				if !strings.HasPrefix(mediaType, entry.kind+"/") {
					return feeds.Feed{}, "", errors.New("媒体类型与声明不一致")
				}
			}
			digest := hex.EncodeToString(contentHash.Sum(nil))
			asset := feeds.OfflineAsset{Snapshot: snapshot, File: file, ContentType: mediaType, Size: size, SHA256: digest, ETag: response.Header.Get("ETag"), LastModified: response.Header.Get("Last-Modified")}
			if hasPrevious && previousAsset.SHA256 != "" && previousAsset.SHA256 == digest && previousAsset.ContentType == mediaType && previousAsset.Size == size {
				if err = os.Remove(path); err != nil {
					return feeds.Feed{}, "", err
				}
				previousAsset.ETag, previousAsset.LastModified = asset.ETag, asset.LastModified
				asset = previousAsset
			}
			downloaded[entry.url] = asset
			item.OfflineMedia[entry.url] = asset
			if entry.kind == "audio" && item.AudioURL == "" {
				item.AudioURL = entry.url
			}
			if entry.kind == "video" && item.VideoURL == "" {
				item.VideoURL = entry.url
			}
		}
	}
	_ = os.Remove(dir) // Only removes an empty snapshot when every file was reused.
	success = true
	return feed, snapshot, nil
}

// Open accepts only regular files within a named owned snapshot.
func Open(root string, asset feeds.OfflineAsset) (*os.File, error) {
	if !snapshotName.MatchString(asset.Snapshot) || !fileName.MatchString(asset.File) || asset.Size < 0 || asset.Size > maxMedia {
		return nil, errors.New("离线媒体标识无效")
	}
	parent, err := mediaRoot(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(parent, asset.Snapshot)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("离线媒体目录无效")
	}
	path := filepath.Join(dir, asset.File)
	info, err = os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return nil, errors.New("离线媒体文件无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("离线媒体文件已更改")
	}
	return f, nil
}
func Render(root string, item feeds.Item) (string, error) {
	content := item.Content
	if content == "" {
		content = item.Description
	}
	return preview.HTMLWithSources(content, func(raw string) ([]byte, string, error) {
		address, err := canonical(item.OfflineBase, raw)
		if err != nil {
			return nil, "", err
		}
		asset, ok := item.OfflineMedia[address]
		if !ok || !strings.HasPrefix(asset.ContentType, "image/") || asset.Size > maxImage {
			return nil, "", errors.New("图片未固定")
		}
		f, err := Open(root, asset)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxImage+1))
		if err != nil || len(data) > int(maxImage) {
			return nil, "", errors.New("图片读取失败")
		}
		return data, asset.ContentType, nil
	})
}

// Discard removes an unpublished snapshot created by this package.
func Discard(root, snapshot string) error {
	if !snapshotName.MatchString(snapshot) {
		return errors.New("离线缓存标识无效")
	}
	parent, err := mediaRoot(root)
	if err != nil {
		return err
	}
	dir := filepath.Join(parent, snapshot)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("离线缓存目录无效")
	}
	return os.RemoveAll(dir)
}
