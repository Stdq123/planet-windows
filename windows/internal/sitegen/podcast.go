package sitegen

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windows/internal/siteimage"
	"windows/internal/store"
)

type podcastEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}
type podcastItem struct {
	Title       string           `xml:"title"`
	Link        string           `xml:"link"`
	GUID        string           `xml:"guid"`
	Description string           `xml:"description"`
	Date        string           `xml:"pubDate"`
	Enclosure   podcastEnclosure `xml:"enclosure"`
	Duration    string           `xml:"itunes:duration,omitempty"`
}
type podcastCategory struct {
	Text string `xml:"text,attr"`
}
type podcastImage struct {
	Href string `xml:"href,attr"`
}
type podcastChannel struct {
	Title       string            `xml:"title"`
	Link        string            `xml:"link"`
	Description string            `xml:"description"`
	Language    string            `xml:"language"`
	Author      string            `xml:"itunes:author,omitempty"`
	Explicit    string            `xml:"itunes:explicit"`
	Image       *podcastImage     `xml:"itunes:image,omitempty"`
	Categories  []podcastCategory `xml:"itunes:category,omitempty"`
	Items       []podcastItem     `xml:"item"`
}

func podcastCategoryMap(categories []string) map[string][]string {
	if len(categories) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, category := range categories {
		out[category] = []string{}
	}
	return out
}

func audioMIME(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	default:
		return "application/octet-stream"
	}
}

func renderPodcast(dir string, site store.Site, articles []store.Article) (bool, error) {
	base := store.CanonicalSiteBase(site.Domain, site.IPNS)
	language := site.PodcastLanguage
	if language == "" {
		language = "en"
	}
	channel := podcastChannel{Title: site.Name, Link: base, Description: site.About, Language: language, Author: site.AuthorName, Explicit: "false"}
	if site.PodcastExplicit {
		channel.Explicit = "true"
	}
	for _, category := range site.PodcastCategories {
		channel.Categories = append(channel.Categories, podcastCategory{Text: category})
	}
	for _, article := range articles {
		if article.ArticleType == 1 || article.AudioFilename == "" {
			continue
		}
		var size int64
		if article.AudioByteLength != nil {
			size = *article.AudioByteLength
		}
		if size < 0 {
			return false, fmt.Errorf("音频文件大小无效")
		}
		file := base + url.PathEscape(article.ID) + "/" + url.PathEscape(article.AudioFilename)
		link := base + strings.TrimPrefix(articleLink(article), "/")
		item := podcastItem{Title: article.Title, Link: link, GUID: link, Description: article.Content, Date: article.Created.UTC().Format(time.RFC1123Z), Enclosure: podcastEnclosure{URL: file, Length: size, Type: audioMIME(article.AudioFilename)}}
		if article.AudioDuration != nil && *article.AudioDuration >= 0 {
			d := *article.AudioDuration
			item.Duration = fmt.Sprintf("%02d:%02d:%02d", d/3600, (d%3600)/60, d%60)
		}
		channel.Items = append(channel.Items, item)
	}
	if site.PodcastCoverPNG != "" && !site.Imported {
		_, data, err := siteimage.DecodePNG(site.PodcastCoverPNG)
		if err != nil {
			return false, err
		}
		if err = os.WriteFile(filepath.Join(dir, "podcastCoverArt.png"), data, 0600); err != nil {
			return false, err
		}
		channel.Image = &podcastImage{Href: base + "podcastCoverArt.png"}
	}
	if len(channel.Items) == 0 {
		return false, nil
	}
	feed := struct {
		XMLName xml.Name       `xml:"rss"`
		Version string         `xml:"version,attr"`
		ITunes  string         `xml:"xmlns:itunes,attr"`
		Channel podcastChannel `xml:"channel"`
	}{Version: "2.0", ITunes: "http://www.itunes.com/dtds/podcast-1.0.dtd", Channel: channel}
	b, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return false, err
	}
	b = append([]byte(xml.Header), b...)
	if err = os.WriteFile(filepath.Join(dir, "podcast.xml"), b, 0600); err != nil {
		return false, err
	}
	return true, nil
}
