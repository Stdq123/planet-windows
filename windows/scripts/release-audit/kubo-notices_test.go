//go:build ignore

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestSupplementIdentityIsExact(t *testing.T) {
	for path, rule := range supplements {
		m := &debug.Module{Path: path, Version: rule.version, Sum: rule.sum}
		if _, err := supplementFor(m, reviewedKuboSHA256); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, field := range []string{"path", "version", "sum", "replacement"} {
			changed := *m
			switch field {
			case "path":
				changed.Path += "/unreviewed"
			case "version":
				changed.Version += "-unreviewed"
			case "sum":
				changed.Sum = "h1:unreviewed"
			case "replacement":
				changed.Replace = m
			}
			if _, err := supplementFor(&changed, reviewedKuboSHA256); err == nil {
				t.Fatalf("accepted changed %s for %s", field, path)
			}
		}
	}
}

func TestNestedClientLicenseScopeRequiresReviewedBinary(t *testing.T) {
	path := "github.com/ipshipyard/p2p-forge"
	rule := supplements[path]
	m := &debug.Module{Path: path, Version: rule.version, Sum: rule.sum}
	for _, hash := range []string{"", strings.Repeat("0", 64)} {
		if _, err := supplementFor(m, hash); err == nil {
			t.Fatal("nested client license accepted for a different binary")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("Unexpected root file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := downloaded{Path: m.Path, Version: m.Version, Dir: dir, Sum: m.Sum}
	if r := collectDownloaded(m, d, "different binary"); r.err == nil {
		t.Fatal("root file bypassed reviewed client scope")
	}
}

func TestSupplementSourceIntegrityAndHeaderBoundary(t *testing.T) {
	dir := t.TempDir()
	text := "// Copyright Fixture\n// License fixture\n\npackage fixture\n"
	file := filepath.Join(dir, "source.go")
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	source := sourceEvidence{"source.go", fmt.Sprintf("%x", sha256.Sum256([]byte(text))), 2}
	rule := supplement{revision: strings.Repeat("a", 40), files: []sourceEvidence{source}}
	m := &debug.Module{Path: "github.com/example/fixture"}
	notices, err := readSupplement(m, dir, rule)
	if err != nil {
		t.Fatal(err)
	}
	output := fmt.Sprint(notices)
	if !strings.Contains(output, "Copyright Fixture") || !strings.Contains(output, "License fixture") || strings.Contains(output, "package fixture") {
		t.Fatalf("wrong header extraction: %s", output)
	}
	if !strings.Contains(output, source.sha256) || !strings.Contains(output, "https://github.com/example/fixture/blob/"+rule.revision+"/source.go") {
		t.Fatal("missing pinned source provenance")
	}
	// Even a change below the extracted header invalidates the reviewed source.
	if err := os.WriteFile(file, []byte(text+"// changed body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSupplement(m, dir, rule); err == nil {
		t.Fatal("accepted modified source")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err := readSupplement(m, dir, rule); err == nil {
		t.Fatal("accepted missing source")
	}
}

func TestUnreviewedMissingRootRemainsUnresolved(t *testing.T) {
	dir := t.TempDir()
	m := &debug.Module{Path: "github.com/example/unreviewed", Version: "v1.0.0", Sum: "h1:fixture"}
	d := downloaded{Path: m.Path, Version: m.Version, Dir: dir, Sum: m.Sum}
	if r := collectDownloaded(m, d, reviewedKuboSHA256); r.err == nil {
		t.Fatal("unknown module without root license was accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("Fixture license\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := collectDownloaded(m, d, ""); r.err != nil || len(r.notices) != 1 {
		t.Fatalf("ordinary root license collection regressed: %+v", r)
	}
	d.Sum = "h1:different"
	if r := collectDownloaded(m, d, reviewedKuboSHA256); r.err == nil {
		t.Fatal("download checksum mismatch accepted")
	}
}

func TestGoLicenseTextAndReviewCoverage(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(goBSDLicense))); got != goBSDLicenseSHA256 {
		t.Fatalf("embedded official license bytes differ: %s", got)
	}
	for _, path := range []string{"github.com/alexbrainman/goissue34681", "github.com/whyrusleeping/base32"} {
		if len(supplements[path].goLicenseRevision) != 40 {
			t.Fatalf("missing immutable Go license provenance: %s", path)
		}
	}
	bbloom := supplements["github.com/ipfs/bbloom"].files
	if len(bbloom) != 2 || bbloom[0].path != "bbloom.go" || bbloom[1].path != "sipHash.go" {
		t.Fatal("MIT and CC0 source evidence must both remain included")
	}
}

func TestFullNestedLicenseAndBSDMappingArePreserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "client"), 0700); err != nil {
		t.Fatal(err)
	}
	text := "License heading\n\nComplete permission terms.\nFinal disclaimer.\n"
	if err := os.WriteFile(filepath.Join(dir, "client", "LICENSE.md"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	rule := supplement{
		revision:          strings.Repeat("b", 40),
		goLicenseRevision: "b17fd8e49d24eb298c53de5cd0a8923f1e0270ba",
		files:             []sourceEvidence{{"client/LICENSE.md", fmt.Sprintf("%x", sha256.Sum256([]byte(text))), 0}},
	}
	notices, err := readSupplement(&debug.Module{Path: "github.com/example/fixture"}, dir, rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 3 || !strings.HasSuffix(notices[1].text, text) || !strings.HasSuffix(notices[2].text, goBSDLicense) {
		t.Fatal("full license or BSD mapping text was lost")
	}
	if !strings.Contains(notices[0].text, "does not approve distribution") || !strings.Contains(notices[2].text, rule.goLicenseRevision+"/LICENSE") {
		t.Fatal("lost review boundary or immutable official text source")
	}
}
