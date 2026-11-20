package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"windows/internal/localapi"
	"windows/internal/store"
)

func TestPNCLIDiskAndAPIMode(t *testing.T) {
	libDir := t.TempDir()

	// 1. Disk mode: create planet & article, search, update, delete
	var out bytes.Buffer
	if err := runCLI([]string{"--source", "disk", "--library", libDir, "--json", "planet", "create", "--name", "CLI Smoke", "--about", "Disk test"}, &out); err != nil {
		t.Fatal(err)
	}
	var createdSite store.Site
	if err := json.Unmarshal(out.Bytes(), &createdSite); err != nil || createdSite.Name != "CLI Smoke" {
		t.Fatalf("unexpected disk created site: %+v (%v)", createdSite, err)
	}

	out.Reset()
	if err := runCLI([]string{"--source", "disk", "--library", libDir, "--json", "article", "create", createdSite.ID[:8], "--title", "Hello CLI", "--content", "World from pn"}, &out); err != nil {
		t.Fatal(err)
	}
	var createdArt store.Article
	if err := json.Unmarshal(out.Bytes(), &createdArt); err != nil || createdArt.Title != "Hello CLI" {
		t.Fatalf("unexpected disk created article: %+v (%v)", createdArt, err)
	}

	out.Reset()
	if err := runCLI([]string{"--source", "disk", "--library", libDir, "--json", "search", "World"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Hello CLI")) {
		t.Fatalf("expected search result to contain Hello CLI: %s", out.String())
	}

	// 2. API mode against a running localapi.Server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	if _, err := localapi.SaveConfig(libDir, true, port, false, "Planet", ""); err != nil {
		t.Fatal(err)
	}
	srv := localapi.NewServer(localapi.Backend{
		OpenStore: func() (*store.Store, error) { return store.Open(libDir) },
	})
	if err := srv.Sync(libDir); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	out.Reset()
	if err := runCLI([]string{"--source", "api", "--api-url", apiURL, "--json", "planet", "list"}, &out); err != nil {
		t.Fatal(err)
	}
	var sites []store.Site
	if err := json.Unmarshal(out.Bytes(), &sites); err != nil || len(sites) != 1 || sites[0].ID != createdSite.ID {
		t.Fatalf("unexpected API planet list: %+v (%v)", sites, err)
	}
}
