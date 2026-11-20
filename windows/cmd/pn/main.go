// pn is Planet Windows' command-line utility supporting --source auto|api|disk.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windows/internal/store"
)

type cliOptions struct {
	jsonOut bool
	library string
	apiURL  string
	source  string
	timeout int
}

func defaultLibraryPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appData, "Planet Windows")
}

func parseGlobalArgs(args []string) (cliOptions, []string, error) {
	opts := cliOptions{
		library: defaultLibraryPath(),
		apiURL:  "http://127.0.0.1:8086",
		source:  "auto",
		timeout: 5,
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json", "--pretty":
			opts.jsonOut = true
		case "--library":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--library requires a path")
			}
			i++
			opts.library = args[i]
		case "--api-url":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--api-url requires a URL")
			}
			i++
			opts.apiURL = strings.TrimRight(args[i], "/")
		case "--source":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--source requires auto|api|disk")
			}
			i++
			opts.source = strings.ToLower(args[i])
			if opts.source != "auto" && opts.source != "api" && opts.source != "disk" {
				return opts, nil, errors.New("invalid --source: expected auto, api, or disk")
			}
		case "--timeout":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--timeout requires seconds")
			}
			i++
		default:
			rest = append(rest, arg)
		}
	}
	return opts, rest, nil
}

func apiRequest(opts cliOptions, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, opts.apiURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	user := os.Getenv("PN_API_USERNAME")
	pass := os.Getenv("PN_API_PASSCODE")
	if user == "" {
		user = "Planet"
	}
	if pass != "" {
		req.SetBasicAuth(user, pass)
	}
	client := &http.Client{Timeout: time.Duration(opts.timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return data, resp.StatusCode, err
}

func resolveSource(opts cliOptions) (string, error) {
	if opts.source == "disk" {
		return "disk", nil
	}
	_, code, err := apiRequest(opts, http.MethodGet, "/v0/ping", nil)
	if err == nil && code == http.StatusOK {
		return "api", nil
	}
	if err == nil && code == http.StatusUnauthorized {
		return "", errors.New("API authentication required: set PN_API_USERNAME and PN_API_PASSCODE")
	}
	if opts.source == "api" {
		return "", errors.New("Planet local REST API is not reachable at " + opts.apiURL)
	}
	return "disk", nil
}

func printOutput(w io.Writer, jsonMode bool, value any, human string) error {
	if jsonMode {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	}
	_, err := fmt.Fprintln(w, human)
	return err
}

func findFlag(args []string, name string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1], true
		}
	}
	return "", false
}

func findDiskSite(s *store.Store, selector string) (store.Site, error) {
	sites, err := s.ListSites()
	if err != nil {
		return store.Site{}, err
	}
	for _, site := range sites {
		if strings.EqualFold(site.ID, selector) || (site.Slug != "" && strings.EqualFold(site.Slug, selector)) || strings.EqualFold(site.Name, selector) {
			return site, nil
		}
	}
	for _, site := range sites {
		if len(selector) >= 4 && strings.HasPrefix(strings.ToLower(site.ID), strings.ToLower(selector)) {
			return site, nil
		}
	}
	return store.Site{}, fmt.Errorf("planet not found: %s", selector)
}

func findDiskArticle(s *store.Store, siteID, selector string) (store.Article, error) {
	articles, err := s.ListArticles(siteID)
	if err != nil {
		return store.Article{}, err
	}
	for _, a := range articles {
		if strings.EqualFold(a.ID, selector) || (a.ArticleReference != "" && strings.EqualFold(a.ArticleReference, selector)) || strings.EqualFold(a.Title, selector) {
			return a, nil
		}
	}
	for _, a := range articles {
		if len(selector) >= 4 && strings.HasPrefix(strings.ToLower(a.ID), strings.ToLower(selector)) {
			return a, nil
		}
	}
	return store.Article{}, fmt.Errorf("article not found: %s", selector)
}

func runCLI(rawArgs []string, out io.Writer) error {
	opts, args, err := parseGlobalArgs(rawArgs)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return printOutput(out, opts.jsonOut, map[string]string{"usage": "pn [--source auto|api|disk] [--library <path>] [--api-url <url>] [--json] <command>"}, "Usage: pn [--source auto|api|disk] [--library <path>] [--api-url <url>] [--json] <planet|article|search|status|version>")
	}
	switch args[0] {
	case "version", "--version":
		return printOutput(out, opts.jsonOut, map[string]string{"version": "pn 0.1.0"}, "pn 0.1.0")
	case "status":
		mode, err := resolveSource(opts)
		if err != nil {
			return err
		}
		return printOutput(out, opts.jsonOut, map[string]string{"source": mode, "library": opts.library, "apiUrl": opts.apiURL}, fmt.Sprintf("source: %s\nlibrary: %s\napi: %s", mode, opts.library, opts.apiURL))
	case "api":
		if len(args) >= 2 && args[1] == "status" {
			_, code, err := apiRequest(opts, http.MethodGet, "/v0/ping", nil)
			running := err == nil && (code == http.StatusOK || code == http.StatusUnauthorized)
			return printOutput(out, opts.jsonOut, map[string]any{"running": running, "status": code}, fmt.Sprintf("running: %v (status %d)", running, code))
		}
		return errors.New("expected: pn api status")
	case "library":
		if len(args) >= 2 && args[1] == "path" {
			return printOutput(out, opts.jsonOut, map[string]string{"path": opts.library}, opts.library)
		}
		return errors.New("expected: pn library path")
	case "planet":
		if len(args) < 2 {
			return errors.New("expected planet subcommand: list, show, create, update, delete, publish")
		}
		mode, err := resolveSource(opts)
		if err != nil {
			return err
		}
		sub := args[1]
		switch sub {
		case "list":
			if mode == "api" {
				data, _, err := apiRequest(opts, http.MethodGet, "/v0/planets/my", nil)
				if err != nil {
					return err
				}
				var sites []store.Site
				if err := json.Unmarshal(data, &sites); err != nil {
					return err
				}
				return printOutput(out, opts.jsonOut, sites, fmt.Sprintf("%d planets", len(sites)))
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			sites, err := s.ListSites()
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, sites, fmt.Sprintf("%d planets", len(sites)))
		case "show":
			if len(args) < 3 {
				return errors.New("pn planet show requires <planet>")
			}
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodGet, "/v0/planets/my/"+url.PathEscape(args[2]), nil)
				if err != nil || code != http.StatusOK {
					return errors.New("planet not found")
				}
				var site store.Site
				_ = json.Unmarshal(data, &site)
				return printOutput(out, opts.jsonOut, site, fmt.Sprintf("%s\t%s", site.ID, site.Name))
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, args[2])
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, site, fmt.Sprintf("%s\t%s", site.ID, site.Name))
		case "create":
			name, _ := findFlag(args[2:], "--name")
			about, _ := findFlag(args[2:], "--about")
			if strings.TrimSpace(name) == "" {
				return errors.New("pn planet create requires --name <name>")
			}
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodPost, "/v0/planets/my", map[string]string{"name": name, "about": about})
				if err != nil || code >= 400 {
					return errors.New("failed to create planet via API")
				}
				var site store.Site
				_ = json.Unmarshal(data, &site)
				return printOutput(out, opts.jsonOut, site, site.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := s.CreateSite(name, about)
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, site, site.ID)
		case "update":
			if len(args) < 3 {
				return errors.New("pn planet update requires <planet>")
			}
			name, _ := findFlag(args[3:], "--name")
			about, _ := findFlag(args[3:], "--about")
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodPost, "/v0/planets/my/"+url.PathEscape(args[2]), map[string]string{"name": name, "about": about})
				if err != nil || code >= 400 {
					return errors.New("failed to update planet via API")
				}
				var site store.Site
				_ = json.Unmarshal(data, &site)
				return printOutput(out, opts.jsonOut, site, site.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, args[2])
			if err != nil {
				return err
			}
			if name == "" {
				name = site.Name
			}
			updated, err := s.UpdateSite(site.ID, name, about)
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, updated, updated.ID)
		case "delete":
			if len(args) < 3 {
				return errors.New("pn planet delete requires <planet>")
			}
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodDelete, "/v0/planets/my/"+url.PathEscape(args[2]), nil)
				if err != nil || code >= 400 {
					return errors.New("failed to delete planet via API")
				}
				var site store.Site
				_ = json.Unmarshal(data, &site)
				return printOutput(out, opts.jsonOut, site, site.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, args[2])
			if err != nil {
				return err
			}
			if err := s.RemoveSite(site.ID); err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, site, site.ID)
		case "publish":
			if len(args) < 3 {
				return errors.New("pn planet publish requires <planet>")
			}
			if mode != "api" {
				return errors.New("planet publish requires the running Planet app API")
			}
			data, code, err := apiRequest(opts, http.MethodPost, "/v0/planets/my/"+url.PathEscape(args[2])+"/publish", nil)
			if err != nil || code >= 400 {
				return errors.New("failed to publish planet via API")
			}
			var site store.Site
			_ = json.Unmarshal(data, &site)
			return printOutput(out, opts.jsonOut, site, site.ID)
		default:
			return fmt.Errorf("unknown planet subcommand: %s", sub)
		}
	case "article":
		if len(args) < 3 {
			return errors.New("expected article subcommand: list, show, create, update, delete")
		}
		mode, err := resolveSource(opts)
		if err != nil {
			return err
		}
		sub := args[1]
		planetSel := args[2]
		switch sub {
		case "list":
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodGet, "/v0/planets/my/"+url.PathEscape(planetSel)+"/articles", nil)
				if err != nil || code >= 400 {
					return errors.New("failed to list articles")
				}
				var list []store.Article
				_ = json.Unmarshal(data, &list)
				return printOutput(out, opts.jsonOut, list, fmt.Sprintf("%d articles", len(list)))
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, planetSel)
			if err != nil {
				return err
			}
			list, err := s.ListArticles(site.ID)
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, list, fmt.Sprintf("%d articles", len(list)))
		case "create":
			title, _ := findFlag(args[3:], "--title")
			content, _ := findFlag(args[3:], "--content")
			if contentFile, ok := findFlag(args[3:], "--content-file"); ok && contentFile != "" {
				b, err := os.ReadFile(contentFile)
				if err != nil {
					return err
				}
				content = string(b)
			}
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodPost, "/v0/planets/my/"+url.PathEscape(planetSel)+"/articles", map[string]string{"title": title, "content": content})
				if err != nil || code >= 400 {
					return errors.New("failed to create article")
				}
				var art store.Article
				_ = json.Unmarshal(data, &art)
				return printOutput(out, opts.jsonOut, art, art.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, planetSel)
			if err != nil {
				return err
			}
			art, err := s.SaveArticle(site.ID, "", title, content)
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, art, art.ID)
		case "update":
			if len(args) < 4 {
				return errors.New("pn article update requires <planet> <article>")
			}
			artSel := args[3]
			title, _ := findFlag(args[4:], "--title")
			content, _ := findFlag(args[4:], "--content")
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodPost, "/v0/planets/my/"+url.PathEscape(planetSel)+"/articles/"+url.PathEscape(artSel), map[string]string{"title": title, "content": content})
				if err != nil || code >= 400 {
					return errors.New("failed to update article")
				}
				var art store.Article
				_ = json.Unmarshal(data, &art)
				return printOutput(out, opts.jsonOut, art, art.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, planetSel)
			if err != nil {
				return err
			}
			art, err := findDiskArticle(s, site.ID, artSel)
			if err != nil {
				return err
			}
			if title == "" {
				title = art.Title
			}
			if content == "" {
				content = art.Content
			}
			updated, err := s.SaveArticle(site.ID, art.ID, title, content)
			if err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, updated, updated.ID)
		case "delete":
			if len(args) < 4 {
				return errors.New("pn article delete requires <planet> <article>")
			}
			artSel := args[3]
			if mode == "api" {
				data, code, err := apiRequest(opts, http.MethodDelete, "/v0/planets/my/"+url.PathEscape(planetSel)+"/articles/"+url.PathEscape(artSel), nil)
				if err != nil || code >= 400 {
					return errors.New("failed to delete article")
				}
				var art store.Article
				_ = json.Unmarshal(data, &art)
				return printOutput(out, opts.jsonOut, art, art.ID)
			}
			s, err := store.Open(opts.library)
			if err != nil {
				return err
			}
			site, err := findDiskSite(s, planetSel)
			if err != nil {
				return err
			}
			art, err := findDiskArticle(s, site.ID, artSel)
			if err != nil {
				return err
			}
			if err := s.DeleteArticle(site.ID, art.ID); err != nil {
				return err
			}
			return printOutput(out, opts.jsonOut, art, art.ID)
		default:
			return fmt.Errorf("unknown article subcommand: %s", sub)
		}
	case "search":
		if len(args) < 2 {
			return errors.New("pn search requires <query>")
		}
		q := args[1]
		mode, err := resolveSource(opts)
		if err != nil {
			return err
		}
		if mode == "api" {
			data, code, err := apiRequest(opts, http.MethodGet, "/v0/search?q="+url.QueryEscape(q), nil)
			if err != nil || code >= 400 {
				return errors.New("search failed")
			}
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			return printOutput(out, opts.jsonOut, res, string(data))
		}
		s, err := store.Open(opts.library)
		if err != nil {
			return err
		}
		articles, err := s.SearchArticles(q)
		if err != nil {
			return err
		}
		res := map[string]any{"articles": articles}
		return printOutput(out, opts.jsonOut, res, fmt.Sprintf("%d matches", len(articles)))
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func main() {
	if err := runCLI(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
