// Package tagname validates public tag filenames shared by storage and rendering.
package tagname

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
)

var pagePattern = regexp.MustCompile(`(?i)^page[0-9]+$`)

// FoldKey conservatively groups the same Unicode simple-fold classes as EqualFold.
func FoldKey(value string) string {
	var folded strings.Builder
	for _, r := range value {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		folded.WriteRune(minimum)
	}
	return folded.String()
}

func Valid(key, label string) bool {
	lower := strings.ToLower(FoldKey(key))
	if key == "" || len(utf16.Encode([]rune(key))) > 250 || pagePattern.MatchString(key) || lower == "index" || lower == "tags" || lower == "archive" || lower == "archives" || len(label) > 4096 || strings.TrimSpace(label) == "" || strings.ContainsAny(label, "\x00\r\n") {
		return false
	}
	upper := strings.ToUpper(key)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return false
	}
	if strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT") {
		suffix := []rune(upper[3:])
		if len(suffix) == 1 && ((suffix[0] >= '1' && suffix[0] <= '9') || suffix[0] == '¹' || suffix[0] == '²' || suffix[0] == '³') {
			return false
		}
	}
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
