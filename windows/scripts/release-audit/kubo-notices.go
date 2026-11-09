//go:build ignore

// Command kubo-notices collects root license files and narrowly reviewed
// supplemental evidence for modules recorded in an official Kubo binary.
// Collection is not legal clearance; other licensing obligations remain.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

type downloaded struct {
	Path, Version, Dir, Sum, Error string
}
type result struct {
	module  *debug.Module
	notices []notice
	err     error
}

type notice struct{ title, text string }
type sourceEvidence struct {
	path, sha256 string
	headerLines  int // Zero includes the complete license file.
}
type supplement struct {
	version, sum, revision, scope string
	files                         []sourceEvidence
	goLicenseRevision             string
	binarySHA256                  string
}

// client/LICENSE.md only covers the p2p-forge client used by this exact official
// Windows/amd64 Kubo 0.38.2 binary. A different build needs a new package-scope
// review even if it contains the same module version.
const reviewedKuboSHA256 = "ac6056b906c4963293ba148e3592ac774e96073b54ba363a6f27d1faedd2bbe3"

var supplements = map[string]supplement{
	"github.com/ipfs/bbloom": {
		version: "v0.0.4", sum: "h1:Gi+8EGJ2y5qiD5FbsbpX/TMNcJw8gSqr7eyjHa4Fhvs=",
		revision: "627bb99eb0f4c49ab3e3276f5574b9b80f8aa3bc",
		scope:    "The runtime source files carry MIT and CC0 notices respectively; both are retained below.",
		files: []sourceEvidence{
			{"bbloom.go", "0ef29cd728f00587bff0c6cd828dd75aa81a23d1eceffe682e02c895ecd1d66c", 19},
			{"sipHash.go", "f22636f6a995548dd2636f1fc3664df7467ba2286ae4c9bdfab573aa2426aaee", 6},
		},
	},
	"github.com/ipshipyard/p2p-forge": {
		version: "v0.6.1", sum: "h1:987/hUC1YxI56CcMX6iTB+9BLjFV0d2SJnig9Z1pf8A=",
		revision: "bdc3d056572588eb4a7f1d379092cabfcaf1fb26", binarySHA256: reviewedKuboSHA256,
		scope: "Scope: p2p-forge/client only; not a license conclusion for the server packages. " +
			"Reviewed Kubo Windows/amd64 CGO_ENABLED=0 dependency graph (go list -mod=readonly -deps ./cmd/ipfs) includes only this package from p2p-forge. " +
			"Kubo imports it in core/node/libp2p/{transport,addrs}.go and config/autotls.go: " +
			"https://github.com/ipfs/kubo/tree/9fd105a1ba37d4b621b5d3384bad92fadac2e6c4 . " +
			"The client does not import other packages in this module. " +
			"Reviewed binary SHA-256: " + reviewedKuboSHA256,
		files: []sourceEvidence{{"client/LICENSE.md", "32788d5c7178ea2b02e9835cd852f2c923aa075aa41923e766a339442c0889da", 0}},
	},
	"github.com/alexbrainman/goissue34681": {
		version: "v0.0.0-20191006012335-3fc7a47baff5", sum: "h1:iW0a5ljuFxkLGPNem5Ui+KBjFJzKg4Fv2fnxe4dvzpM=",
		revision: "3fc7a47baff5c3bb1a1bf7b6a424cf84ff7f3329",
		scope: "The source retains Go Authors BSD-style headers but omits the referenced LICENSE. " +
			"The Go BSD text below accompanies those existing declarations; this is an explicit provenance mapping, not a new grant.",
		goLicenseRevision: "b17fd8e49d24eb298c53de5cd0a8923f1e0270ba", // Go 1.13.1
		files: []sourceEvidence{
			{"doc.go", "0667037d0b121dcfe7d8943b223dceed03c3ae99e2e28f95a20fe753e768bcbf", 3},
			{"main.go", "6b4677f812fc60b5a9001644c3b9ca9ba8a3be0c6160af29a2303c631b9bb158", 3},
			{"main_windows.go", "4255652050d20da868bd15b0902a0c4e3df423404859a6a65ad8f5f21a1aacc4", 3},
		},
	},
	"github.com/whyrusleeping/base32": {
		version: "v0.0.0-20170828182744-c30ac30633cc", sum: "h1:BCPnHtcboadS0DvysUuJXZ4lWVv5Bh5i7+tbIyi+ck4=",
		revision: "c30ac30633ccdabefe87eb12465113f06f1bab75",
		scope: "The source retains a Go Authors BSD-style header but omits the referenced LICENSE. " +
			"The Go BSD text below accompanies that existing declaration; this is an explicit provenance mapping, not a new grant.",
		goLicenseRevision: "352996a381701cfa0c16e8de29cbde8f3922182f", // Go 1.8.3
		files:             []sourceEvidence{{"base32.go", "46f2bf3437fe2e260f8ab914d9b70d7a8c0c406ba43b792bddc0d6d32b8ae54c", 3}},
	},
}

// The immutable Go revisions above contain these identical bytes. Keep the
// complete upstream text, including its copyright, conditions and disclaimer.
const goBSDLicenseSHA256 = "2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067"
const goBSDLicense = `Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
`

func supplementFor(m *debug.Module, binarySHA256 string) (supplement, error) {
	rule, ok := supplements[m.Path]
	if !ok || m.Replace != nil || m.Version != rule.version || m.Sum != rule.sum {
		return supplement{}, errors.New("no root license; no reviewed supplement for this exact module identity")
	}
	if rule.binarySHA256 != "" && binarySHA256 != rule.binarySHA256 {
		return supplement{}, errors.New("nested client license scope is not reviewed for this binary SHA-256")
	}
	return rule, nil
}

func readSupplement(m *debug.Module, dir string, rule supplement) ([]notice, error) {
	notices := []notice{{"Supplemental collection evidence", "Missing-root-file collection gap only; this does not approve distribution or settle other license obligations.\n" + rule.scope}}
	for _, source := range rule.files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(source.path)))
		if err != nil {
			return nil, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != source.sha256 {
			return nil, fmt.Errorf("reviewed source hash differs: %s", source.path)
		}
		text := string(data)
		if source.headerLines > 0 {
			lines := strings.Split(text, "\n")
			if len(lines) < source.headerLines {
				return nil, fmt.Errorf("reviewed header is truncated: %s", source.path)
			}
			text = strings.Join(lines[:source.headerLines], "\n") + "\n"
		}
		provenance := "Source: https://" + m.Path + "/blob/" + rule.revision + "/" + source.path + "\nSource file SHA-256: " + source.sha256 + "\n\n"
		notices = append(notices, notice{source.path, provenance + text})
	}
	if rule.goLicenseRevision != "" {
		if fmt.Sprintf("%x", sha256.Sum256([]byte(goBSDLicense))) != goBSDLicenseSHA256 {
			return nil, errors.New("embedded Go license bytes differ from reviewed evidence")
		}
		provenance := "Source: https://github.com/golang/go/blob/" + rule.goLicenseRevision + "/LICENSE\nSource file SHA-256: " + goBSDLicenseSHA256 + "\n\n"
		notices = append(notices, notice{"Go BSD LICENSE (source header provenance mapping)", provenance + goBSDLicense})
	}
	return notices, nil
}

var noticeName = regexp.MustCompile(`(?i)^(license|licence|copying|notice|unlicense)([._-].*)?$`)

func collect(m *debug.Module, binarySHA256 string) result {
	r := result{module: m}
	if m.Replace != nil {
		r.err = errors.New("replacement module requires manual review")
		return r
	}
	if m.Version == "" || m.Sum == "" {
		r.err = errors.New("module version or checksum missing")
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "mod", "download", "-json", m.Path+"@"+m.Version)
	out, err := cmd.Output()
	if err != nil {
		r.err = fmt.Errorf("go mod download: %w", err)
		return r
	}
	var d downloaded
	if err := json.Unmarshal(out, &d); err != nil {
		r.err = err
		return r
	}
	return collectDownloaded(m, d, binarySHA256)
}

func collectDownloaded(m *debug.Module, d downloaded, binarySHA256 string) result {
	r := result{module: m}
	if m.Replace != nil || m.Version == "" || m.Sum == "" {
		r.err = errors.New("unversioned or replacement module requires manual review")
		return r
	}
	if d.Error != "" || d.Dir == "" {
		r.err = fmt.Errorf("download error: %s", d.Error)
		return r
	}
	if d.Path != m.Path || d.Version != m.Version || d.Sum != m.Sum {
		r.err = errors.New("downloaded module identity or checksum differs from binary metadata")
		return r
	}
	// Reviewed gaps always require their exact evidence. An unexpected root
	// file in a module cache must not bypass the pins or the client-only scope.
	if _, reviewed := supplements[m.Path]; reviewed {
		rule, err := supplementFor(m, binarySHA256)
		if err != nil {
			r.err = err
			return r
		}
		r.notices, r.err = readSupplement(m, d.Dir, rule)
		return r
	}
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		r.err = err
		return r
	}
	for _, entry := range entries {
		if !entry.IsDir() && noticeName.MatchString(entry.Name()) {
			data, err := os.ReadFile(filepath.Join(d.Dir, entry.Name()))
			if err != nil {
				r.err = err
				return r
			}
			r.notices = append(r.notices, notice{entry.Name(), string(data)})
		}
	}
	if len(r.notices) == 0 {
		r.err = errors.New("no root license or reviewed supplement for this module")
	}
	return r
}

func run() error {
	binary := flag.String("binary", "resources/windows-amd64/ipfs.exe", "Kubo binary")
	output := flag.String("output", "", "output notice file")
	workers := flag.Int("workers", 8, "maximum concurrent module downloads")
	flag.Parse()
	if *output == "" || *workers < 1 || *workers > 16 {
		return errors.New("provide -output and 1–16 workers")
	}
	file, err := os.Open(*binary)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := buildinfo.Read(file)
	if err != nil {
		return err
	}
	if info.Main.Path != "github.com/ipfs/kubo" {
		return errors.New("binary is not Kubo")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	binarySHA256 := fmt.Sprintf("%x", hash.Sum(nil))
	jobs := make(chan *debug.Module)
	results := make(chan result, len(info.Deps))
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				results <- collect(m, binarySHA256)
			}
		}()
	}
	go func() {
		for _, m := range info.Deps {
			jobs <- m
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	var all []result
	for r := range results {
		all = append(all, r)
		if len(all)%25 == 0 {
			fmt.Fprintf(os.Stderr, "Checked %d/%d Kubo modules\n", len(all), len(info.Deps))
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].module.Path < all[j].module.Path })
	var out bytes.Buffer
	fmt.Fprintf(&out, "Kubo %s linked Go module license evidence (%s)\n", info.Main.Version, info.GoVersion)
	fmt.Fprintf(&out, "Binary SHA-256: %s\n", binarySHA256)
	out.WriteString("Root license texts and exact-identity supplemental evidence; collection is not distribution approval. Other nested licensing, source/relinking and non-Go obligations still require independent review.\n")
	missing := 0
	for _, r := range all {
		fmt.Fprintf(&out, "\n========================================================================\n%s %s %s\n", r.module.Path, r.module.Version, r.module.Sum)
		if r.err != nil {
			missing++
			fmt.Fprintf(&out, "REQUIRES MANUAL REVIEW: %v\n", r.err)
			continue
		}
		for _, notice := range r.notices {
			fmt.Fprintf(&out, "\n--- %s ---\n", notice.title)
			out.WriteString(notice.text)
			out.WriteByte('\n')
		}
	}
	if err := os.WriteFile(*output, out.Bytes(), 0600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Collected %d modules; %d require manual review.\n", len(all), missing)
	if missing > 0 {
		return fmt.Errorf("%d modules require manual review; see output", missing)
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
