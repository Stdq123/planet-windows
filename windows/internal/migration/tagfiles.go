package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"windows/internal/backup"
)

// Only slots proven by rendering the verified template with a fresh marker
// may vary. Body text and arbitrary attributes remain exact byte comparisons.
func sameSepiaTimestampPage(archived, regenerated, marked, marker string) bool {
	read := func(path string) ([]byte, error) {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return nil, errors.New("页面无效")
		}
		return os.ReadFile(path)
	}
	old, err := read(archived)
	if err != nil {
		return false
	}
	expected, err := read(regenerated)
	if err != nil {
		return false
	}
	withMarker, err := read(marked)
	if err != nil || marker == "" || bytes.Contains(expected, []byte(marker)) {
		return false
	}
	parts := bytes.Split(withMarker, []byte(marker))
	if len(parts) < 2 || !bytes.Equal(bytes.Join(parts, nil), expected) {
		return false
	}
	stamp := ""
	for i, part := range parts {
		if !bytes.HasPrefix(old, part) {
			return false
		}
		old = old[len(part):]
		if i == len(parts)-1 {
			return len(old) == 0
		}
		n := 0
		for n < len(old) && n < 11 && old[n] >= '0' && old[n] <= '9' {
			n++
		}
		if n == 0 || (stamp != "" && stamp != string(old[:n])) {
			return false
		}
		stamp = string(old[:n])
		old = old[n:]
	}
	return false
}

// renderOriginalTagPages is only evidence for byte comparison; its failure
// never authorizes replacing an archived file. No original data is modified.
func renderOriginalTagPages(archive, destination string, raw json.RawMessage, templateDir, marker string) error {
	parsed, err := backup.ParseMetadata(raw)
	if err != nil {
		return err
	}
	var original, selected struct {
		TemplateName string `json:"templateName"`
		Name         string `json:"name"`
	}
	if json.Unmarshal(raw, &original) != nil {
		return errors.New("原模板信息无效")
	}
	b, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(b, &selected) != nil || original.TemplateName == "" || original.TemplateName != selected.Name {
		return errors.New("原模板与当前模板不一致")
	}
	return stageImportedPublic(archive, destination, raw, parsed.Site, parsed.Articles, templateDir, marker)
}

// Identical bytes can be retained without silently discarding unknown data.
// Both paths are in private staging trees owned by this export operation.
func sameRegularFiles(first, second string) (bool, error) {
	a, err := os.Lstat(first)
	if err != nil || !a.Mode().IsRegular() {
		return false, err
	}
	b, err := os.Lstat(second)
	if err != nil || !b.Mode().IsRegular() || a.Size() != b.Size() {
		return false, err
	}
	hash := func(path string) ([sha256.Size]byte, error) {
		var result [sha256.Size]byte
		f, err := os.Open(path)
		if err != nil {
			return result, err
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			return result, err
		}
		copy(result[:], h.Sum(nil))
		return result, nil
	}
	x, err := hash(first)
	if err != nil {
		return false, err
	}
	y, err := hash(second)
	if err != nil {
		return false, err
	}
	return x == y, nil
}
