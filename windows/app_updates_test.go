package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/egoist/mygo"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"windows/internal/updates"
)

type afterDownloadTransport struct {
	base               http.RoundTripper
	published, release chan struct{}
}

func (t *afterDownloadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, e := t.base.RoundTrip(r)
	if e == nil && r.URL.Path == "/setup.exe" {
		resp.Body = &afterDownloadBody{ReadCloser: resp.Body, published: t.published, release: t.release}
	}
	return resp, e
}

type afterDownloadBody struct {
	io.ReadCloser
	published, release chan struct{}
}

func (b *afterDownloadBody) Close() error {
	e := b.ReadCloser.Close()
	close(b.published)
	<-b.release
	return e
}
func TestTrustChangeAfterPublishingDownloadCleansOnlyOwnedCache(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	body := []byte("verified installer")
	sum := sha256.Sum256(body)
	var envelope []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setup.exe" {
			w.Write(body)
		} else {
			w.Write(envelope)
		}
	}))
	defer server.Close()
	published, release := make(chan struct{}), make(chan struct{})
	old := http.DefaultTransport
	http.DefaultTransport = &afterDownloadTransport{server.Client().Transport, published, release}
	t.Cleanup(func() { http.DefaultTransport = old })
	r := updates.Release{Schema: 1, App: updates.AppID, Platform: "windows-amd64", Version: "0.4.0", MinimumVersion: appVersion, URL: server.URL + "/setup.exe", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)), PublishedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}
	payload, _ := json.Marshal(r)
	envelope, _ = json.Marshal(map[string][]byte{"payload": payload, "signature": ed25519.Sign(key, payload)})
	lib := &Library{}
	if _, e := lib.SaveUpdateSettings(server.URL+"/feed.json", base64.StdEncoding.EncodeToString(pub), false); e != nil {
		t.Fatal(e)
	}
	info, e := lib.CheckUpdates()
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := lib.DownloadUpdate(info.Version, info.SHA256); done <- e }()
	select {
	case <-published:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("download was not published")
	}
	if _, e := lib.SaveUpdateSettings("https://new.example/feed.json", base64.StdEncoding.EncodeToString(pub), false); e != nil {
		close(release)
		t.Fatal(e)
	}
	close(release)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("stale download succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled download stuck")
	}
	filepath.Walk(filepath.Join(root, "updates"), func(path string, info os.FileInfo, e error) error {
		if e == nil && !info.IsDir() {
			t.Errorf("unreported stale cache remains: %s", path)
		}
		return nil
	})
}

func TestChangingUpdateTrustCancelsOldCheckWithoutRestoringOldSettings(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	entered := make(chan struct{})
	aborted := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(aborted) }))
	defer server.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	lib := &Library{}
	if _, e := lib.SaveUpdateSettings(server.URL+"/feed.json", base64.StdEncoding.EncodeToString(pub), false); e != nil {
		t.Fatal(e)
	}
	checked := make(chan error, 1)
	go func() { _, e := lib.CheckUpdates(); checked <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("check did not start")
	}
	saved := make(chan error, 1)
	go func() {
		_, e := lib.SaveUpdateSettings("https://new.example/feed.json", base64.StdEncoding.EncodeToString(pub), false)
		saved <- e
	}()
	select {
	case e := <-saved:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("trust change blocked on old network request")
	}
	select {
	case e := <-checked:
		if e == nil {
			t.Fatal("stale check succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old check not canceled")
	}
	select {
	case <-aborted:
	case <-time.After(3 * time.Second):
		t.Fatal("network request remained active")
	}
	cfg, e := updates.LoadConfig(root)
	if e != nil || cfg.FeedURL != "https://new.example/feed.json" || cfg.HighestVersion != "" {
		t.Fatal(cfg, e)
	}
	if lib.checkedUpdate != nil {
		t.Fatal("old signed release restored")
	}
	_ = key
}

func TestUpdateCheckPersistsSeenVersionAndRejectsStaleDownload(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	var envelope []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(envelope) }))
	defer server.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	r := updates.Release{Schema: 1, App: updates.AppID, Platform: "windows-amd64", Version: "0.4.0", MinimumVersion: appVersion, URL: server.URL + "/setup.exe", SHA256: strings.Repeat("a", 64), Size: 10, PublishedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}
	payload, _ := json.Marshal(r)
	envelope, _ = json.Marshal(map[string][]byte{"payload": payload, "signature": ed25519.Sign(key, payload)})
	lib := &Library{}
	if _, e := lib.SaveUpdateSettings(server.URL+"/feed.json", base64.StdEncoding.EncodeToString(pub), false); e != nil {
		t.Fatal(e)
	}
	info, e := lib.CheckUpdates()
	if e != nil || !info.Available || info.Version != "0.4.0" {
		t.Fatal(info, e)
	}
	cfg, e := updates.LoadConfig(root)
	if e != nil || cfg.HighestVersion != "0.4.0" {
		t.Fatal(cfg, e)
	}
	lib.CancelUpdateOperation()
	if _, e = lib.DownloadUpdate(info.Version, info.SHA256); e == nil {
		t.Fatal("canceled check authorized download")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = updates.NewClient().Check(ctx, cfg, appVersion, time.Now()); e == nil {
		t.Fatal("canceled context accepted")
	}
}

func TestPrereleaseAppRecordsFullVersionAndRejectsReplay(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	previousVersion := appVersion
	appVersion = "0.0.1-alpha.1"
	t.Cleanup(func() {
		mygo.App.SetPath(mygo.PathUserData, "")
		appVersion = previousVersion
	})
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	var envelope []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(envelope) }))
	defer server.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	lib := &Library{}
	settings, err := lib.SaveUpdateSettings(server.URL+"/feed.json", base64.StdEncoding.EncodeToString(pub), false)
	if err != nil || settings.CurrentVersion != appVersion {
		t.Fatalf("current prerelease version: %+v, %v", settings, err)
	}
	r := updates.Release{Schema: 1, App: updates.AppID, Platform: "windows-amd64", MinimumVersion: appVersion, URL: server.URL + "/setup.exe", SHA256: strings.Repeat("a", 64), Size: 10, PublishedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}
	for _, next := range []string{"0.0.1-alpha.10", "0.0.1-alpha.2", "0.0.1"} {
		r.Version = next
		payload, _ := json.Marshal(r)
		envelope, _ = json.Marshal(map[string][]byte{"payload": payload, "signature": ed25519.Sign(key, payload)})
		info, err := lib.CheckUpdates()
		cfg, configErr := updates.LoadConfig(root)
		if configErr != nil {
			t.Fatal(configErr)
		}
		if next == "0.0.1-alpha.2" {
			if err == nil || cfg.HighestVersion != "0.0.1-alpha.10" || lib.checkedUpdate != nil {
				t.Fatalf("replayed alpha changed trusted state: %+v, %v", cfg, err)
			}
			continue
		}
		if err != nil || !info.Available || info.Version != next || cfg.HighestVersion != next {
			t.Fatalf("prerelease/stable update: %+v, %+v, %v", info, cfg, err)
		}
	}
}
