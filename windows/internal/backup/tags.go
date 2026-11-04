package backup

import (
	"encoding/json"
	"errors"
	"strings"

	"windows/internal/store"
	"windows/internal/tagname"
)

func TagFoldKey(value string) string  { return tagname.FoldKey(value) }
func ValidTag(key, label string) bool { return tagname.Valid(key, label) }

// ImportedTags validates every original key, including keys on deleted posts,
// before constructing the labels and article tags for the current snapshot.
func ImportedTags(raw json.RawMessage, articles []store.Article) (map[string]string, map[string]map[string]string, error) {
	var original struct {
		Tags     map[string]string `json:"tags"`
		Articles []struct {
			ID   string            `json:"id"`
			Tags map[string]string `json:"tags"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &original); err != nil {
		return nil, nil, err
	}
	seen := map[string]string{}
	validate := func(tags map[string]string) error {
		for key, label := range tags {
			lower := TagFoldKey(key)
			if !ValidTag(key, label) {
				return errors.New("原标签名称或路径无效")
			}
			if old, ok := seen[lower]; ok && old != key {
				return errors.New("原标签路径存在大小写冲突")
			}
			seen[lower] = key
		}
		return nil
	}
	if err := validate(original.Tags); err != nil {
		return nil, nil, err
	}
	byID := map[string]map[string]string{}
	for _, article := range original.Articles {
		if err := validate(article.Tags); err != nil {
			return nil, nil, err
		}
		byID[strings.ToUpper(article.ID)] = article.Tags
	}
	for _, article := range articles {
		if article.Tags != nil {
			if err := validate(article.Tags); err != nil {
				return nil, nil, err
			}
		}
	}
	labels := map[string]string{}
	edited := map[string]bool{}
	active := map[string]map[string]string{}
	for _, article := range articles {
		tags := article.Tags
		if tags == nil {
			tags = byID[strings.ToUpper(article.ID)]
		}
		if len(tags) == 0 {
			continue
		}
		active[strings.ToUpper(article.ID)] = tags
		for key, label := range tags {
			oldLabel, hadOld := byID[strings.ToUpper(article.ID)][key]
			isEdited := article.Tags != nil && (!hadOld || oldLabel != label)
			if !isEdited {
				if preferred, ok := original.Tags[key]; ok {
					label = preferred
				}
			}
			if _, ok := labels[key]; !ok || (isEdited && !edited[key]) {
				labels[key] = label
				edited[key] = isEdited
			}
		}
	}
	return labels, active, nil
}
