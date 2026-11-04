//go:build ignore

// Command kubo-notices collects root license files for the Go modules recorded
// in an official Kubo binary. It is an evidence collection aid, not a legal
// analysis; nested licenses and non-Go dependencies still require review.
package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	module *debug.Module
	files  []string
	err    error
}

var noticeName = regexp.MustCompile(`(?i)^(license|licence|copying|notice|unlicense)([._-].*)?$`)

func collect(m *debug.Module) result {
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
	if d.Error != "" || d.Dir == "" {
		r.err = fmt.Errorf("download error: %s", d.Error)
		return r
	}
	if d.Path != m.Path || d.Version != m.Version || d.Sum != m.Sum {
		r.err = errors.New("downloaded module identity or checksum differs from binary metadata")
		return r
	}
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		r.err = err
		return r
	}
	for _, entry := range entries {
		if !entry.IsDir() && noticeName.MatchString(entry.Name()) {
			r.files = append(r.files, filepath.Join(d.Dir, entry.Name()))
		}
	}
	sort.Strings(r.files)
	if len(r.files) == 0 {
		r.err = errors.New("no root license or notice file")
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
	info, err := buildinfo.ReadFile(*binary)
	if err != nil {
		return err
	}
	if info.Main.Path != "github.com/ipfs/kubo" {
		return errors.New("binary is not Kubo")
	}
	jobs := make(chan *debug.Module)
	results := make(chan result, len(info.Deps))
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				results <- collect(m)
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
	out.WriteString("Root license texts only; nested licensing, binary sources and other obligations require independent review.\n")
	missing := 0
	for _, r := range all {
		fmt.Fprintf(&out, "\n========================================================================\n%s %s %s\n", r.module.Path, r.module.Version, r.module.Sum)
		if r.err != nil {
			missing++
			fmt.Fprintf(&out, "REQUIRES MANUAL REVIEW: %v\n", r.err)
			continue
		}
		for _, path := range r.files {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&out, "\n--- %s ---\n", filepath.Base(path))
			out.Write(b)
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
