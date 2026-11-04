package sitegen

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
)

var sepiaColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var sepiaDefaults = map[string]string{
	"showRef": "false", "highlightColor": "#FF4500", "linkColor": "#778087", "linkHoverColor": "#4D5256",
	"siteTitleColor": "#333333", "siteTitleHoverColor": "#000000", "backgroundColor": "#F4F1EC",
	"foregroundColor": "#000000", "foregroundSecondaryColor": "#999999", "borderColor": "#E7E5DF",
	"navigationLinkColor": "#333333", "navigationActiveItemColor": "#FCFCFA",
}

// NormalizeSettings accepts only settings whose exact HTML/CSS behavior is supported.
func NormalizeSettings(template string, raw map[string]json.RawMessage) (map[string]string, error) {
	result := map[string]string{}
	if template != "Sepia" {
		if len(raw) != 0 {
			return nil, errors.New("当前模板的非空设置暂不支持安全重建")
		}
		return result, nil
	}
	for key, value := range raw {
		if _, ok := sepiaDefaults[key]; !ok {
			return nil, errors.New("模板包含不支持的设置：" + key)
		}
		var v string
		if json.Unmarshal(value, &v) != nil {
			if key != "showRef" {
				return nil, errors.New("模板颜色设置无效")
			}
			var b bool
			if json.Unmarshal(value, &b) != nil {
				return nil, errors.New("模板开关设置无效")
			}
			v = strconv.FormatBool(b)
		}
		if key == "showRef" {
			if v != "true" && v != "false" {
				return nil, errors.New("模板开关设置无效")
			}
		} else if !sepiaColor.MatchString(v) {
			return nil, errors.New("模板颜色必须使用六位十六进制格式")
		}
		result[key] = v
	}
	return result, nil
}

func settingsContext(template string, values map[string]string) (map[string]any, error) {
	raw := map[string]json.RawMessage{}
	for key, value := range values {
		b, _ := json.Marshal(value)
		raw[key] = b
	}
	normalized, err := NormalizeSettings(template, raw)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if template == "Sepia" {
		for k, v := range sepiaDefaults {
			result[k] = v
		}
	}
	for k, v := range normalized {
		result[k] = v
	}
	return result, nil
}

func SepiaSettings(values map[string]string) (map[string]string, error) {
	ctx, err := settingsContext("Sepia", values)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(ctx))
	for k, v := range ctx {
		result[k] = v.(string)
	}
	return result, nil
}
