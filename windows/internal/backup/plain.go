package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"windows/internal/store"
)

var unsupportedMarkup = regexp.MustCompile(`(?i)<[a-z!/][^>]*>|https?://(?:www\.)?(?:youtube\.com|youtu\.be)/`)

func populated(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

// CheckSimplePlain admits only fields whose rendering has been implemented.
// Unknown populated fields are rejected, even though they remain in the backup.
func CheckSimplePlain(original json.RawMessage, site store.Site, articles []store.Article) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(original, &root); err != nil || root == nil {
		return errors.New("备份元数据无效")
	}
	var p rawPlanet
	if err := json.Unmarshal(original, &p); err != nil {
		return err
	}
	if p.TemplateName != "Plain" || p.Name != site.Name || p.About != site.About || len(p.Articles) != len(articles) {
		return errors.New("当前只能重建未更改站点设置和文章列表的 Plain 备份")
	}
	if unsupportedMarkup.MatchString(site.About) {
		return errors.New("站点简介包含暂不支持重建的内容")
	}
	allowedSite := map[string]bool{"id": true, "name": true, "about": true, "ipns": true, "created": true, "updated": true, "templateName": true, "articles": true, "lastPublished": true, "lastPublishedCID": true, "nextArticleNumber": true, "publishAsIPNS": true}
	knownSite := strings.Fields("domain authorName slug archived archivedAt plausibleEnabled plausibleDomain plausibleAPIKey plausibleAPIServer twitterUsername githubUsername telegramUsername mastodonUsername discordLink dWebServicesEnabled dWebServicesDomain dWebServicesAPIKey pinnableEnabled pinnableAPIEndpoint pinnablePinCID filebaseEnabled filebasePinName filebaseAPIToken filebaseRequestID filebasePinCID customCodeHeadEnabled customCodeHead customCodeBodyStartEnabled customCodeBodyStart customCodeBodyEndEnabled customCodeBodyEnd podcastCategories podcastLanguage podcastExplicit juiceboxEnabled juiceboxProjectID juiceboxProjectIDGoerli acceptsDonation acceptsDonationMessage acceptsDonationETHAddress tags aggregation reuseOriginalID saveRoundAvatar doNotIndex prewarmNewPost sshRsyncEnabled sshRsyncDestination sshRsyncKeyPath sshRsyncDeleteEnabled cloudflarePagesEnabled cloudflarePagesAccountID cloudflarePagesAPIToken cloudflarePagesProjectName cloudflarePagesLastDeployedProjectName cloudflarePagesLastDeployedURL")
	knownSiteSet := map[string]bool{}
	for _, key := range knownSite {
		knownSiteSet[key] = true
	}
	for key, value := range root {
		if key == "filebaseAPIToken" && (len(root["filebaseEnabled"]) == 0 || !populated(root["filebaseEnabled"])) {
			continue
		}
		if !allowedSite[key] && (!knownSiteSet[key] || populated(value)) {
			return fmt.Errorf("站点设置 %s 暂不支持重建", key)
		}
	}
	var old []map[string]json.RawMessage
	if err := json.Unmarshal(root["articles"], &old); err != nil {
		return err
	}
	byID := map[string]map[string]json.RawMessage{}
	for _, entry := range old {
		var id string
		if json.Unmarshal(entry["id"], &id) != nil || !uuidPattern.MatchString(id) {
			return errors.New("文章标识无效")
		}
		byID[strings.ToUpper(id)] = entry
	}
	allowedArticle := map[string]bool{"id": true, "link": true, "title": true, "content": true, "created": true, "modified": true, "starType": true, "starred": true, "contentRendered": true, "articleNumber": true, "attachments": true}
	knownArticle := strings.Fields("articleType slug heroImage heroImageWidth heroImageHeight externalLink summary videoFilename audioFilename attachments cids tags isIncludedInNavigation navigationWeight originalSiteName originalSiteDomain originalPostID originalPostDate pinned")
	knownArticleSet := map[string]bool{}
	for _, key := range knownArticle {
		knownArticleSet[key] = true
	}
	for _, article := range articles {
		entry, ok := byID[strings.ToUpper(article.ID)]
		if !ok || article.SiteID != site.ID {
			return errors.New("文章列表已更改，暂不支持重建")
		}
		var link string
		if json.Unmarshal(entry["link"], &link) != nil || !strings.EqualFold(link, "/"+article.ID+"/") {
			return errors.New("文章自定义路径暂不支持重建")
		}
		for key, value := range entry {
			if !allowedArticle[key] && (!knownArticleSet[key] || populated(value)) {
				return fmt.Errorf("文章设置 %s 暂不支持重建", key)
			}
		}
		if unsupportedMarkup.MatchString(article.Content) {
			return errors.New("文章包含暂不支持重建的 HTML 或视频链接")
		}
	}
	return nil
}

// CheckSimplePublicArticle rejects public metadata that the rebuilt page cannot represent.
func CheckSimplePublicArticle(fields map[string]json.RawMessage) error {
	allowed := map[string]bool{"id": true, "link": true, "title": true, "content": true, "contentRendered": true, "created": true, "modified": true, "articleNumber": true, "articleReference": true, "attachments": true}
	known := strings.Fields("articleType slug externalLink hasVideo videoFilename hasAudio audioFilename audioDuration audioByteLength attachments heroImage heroImageWidth heroImageHeight heroImageURL heroImageFilename cids tags originalSiteName originalSiteDomain originalPostID originalPostDate pinned")
	knownSet := map[string]bool{}
	for _, key := range known {
		knownSet[key] = true
	}
	for key, value := range fields {
		if !allowed[key] && (!knownSet[key] || populated(value)) {
			return fmt.Errorf("公开文章设置 %s 暂不支持重建", key)
		}
	}
	return nil
}
