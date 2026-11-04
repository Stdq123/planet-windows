package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/attachments"
	"windows/internal/ipfs"
	"windows/internal/migration"
	"windows/internal/offline"
	"windows/internal/services"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

func TestDisablePlausibleWithUnreadableCredentialStillDisablesTracking(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureService(site.ID, "plausible", "example.com", "secret"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "integrations", "plausible", site.ID+".dat"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureService(site.ID, "plausible", "", ""); err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || got.PlausibleOverride == nil || got.PlausibleOverride.Enabled {
		t.Fatalf("tracking still enabled: %+v %v", got, err)
	}
}

func TestConfigurePlausibleStoresValidatedSiteScript(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "secret", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.ScriptID != "pa-Test123" {
		t.Fatalf("missing ID: %+v %v", info, err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "other", `pa-x\"><script>`); err == nil {
		t.Fatal("accepted unsafe script ID")
	}
	info, err = lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.ScriptID != "pa-Test123" {
		t.Fatalf("valid config changed: %+v %v", info, err)
	}
}

func TestPlausibleServiceInfoShowsRestoredTrackingWithoutCredential(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPlausibleTrackingScript(site.ID, true, "example.com", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.Configured || info.Name != "example.com" || info.ScriptID != "pa-Test123" {
		t.Fatalf("missing restored settings: %+v %v", info, err)
	}
}

func TestConfigurePlausibleTrackingDoesNotRequireOrEraseAPIKey(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.Configured || info.Name != "example.com" || info.ScriptID != "pa-Test123" {
		t.Fatalf("tracking without API: %+v %v", info, err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "secret", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "", "pa-New456"); err != nil {
		t.Fatal(err)
	}
	info, err = lib.ServiceInfo(site.ID, "plausible")
	if err != nil || !info.Configured || info.ScriptID != "pa-New456" {
		t.Fatalf("key was lost: %+v %v", info, err)
	}
}

func TestConfigureSelfHostedPlausibleDoesNotMoveSavedTokenSilently(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "old-key", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausibleAt(site.ID, "example.com", "", "pa-Test123", "https://stats.example.com:8443"); err == nil {
		t.Fatal("moved a saved token to another server")
	}
	if err = lib.ConfigurePlausibleAt(site.ID, "example.com", "new-key", "pa-Test123", "https://stats.example.com:8443"); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.Server != "https://stats.example.com:8443" {
		t.Fatalf("wrong server: %+v %v", info, err)
	}
	cfg, err := services.LoadConfig(root, site.ID, "plausible")
	if err != nil || cfg.Token != "new-key" || cfg.Server != "https://stats.example.com:8443" {
		t.Fatalf("wrong saved credentials: %+v %v", cfg, err)
	}
}

func TestPlausibleStatsRefusesCredentialsWhenImportedServerDiffers(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Imported", IPNS: "k51example", Imported: true}
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Imported","articles":[],"plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"plausible.io"}`)
	if err = s.Import(site, nil, raw); err != nil {
		t.Fatal(err)
	}
	if err = services.SaveConfig(root, site.ID, "plausible", services.Config{Name: "example.com", Token: "private", Server: "https://stats.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.PlausibleStats(site.ID); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("expected mismatch without network, got %v", err)
	}
}

func TestPlausibleStatsRejectsInvalidRangeBeforeNetworking(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausible(site.ID, "example.com", "secret", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.PlausibleStatsRange(site.ID, "not-a-range"); err == nil || !strings.Contains(err.Error(), "时间范围") {
		t.Fatalf("invalid period accepted: %v", err)
	}
}

func TestPlausibleServiceInfoShowsOriginalImportedTracking(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Imported", IPNS: "k51example", Imported: true}
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Imported","articles":[],"plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"plausible.io"}`)
	if err = s.Import(site, nil, raw); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || !info.TrackingEnabled || info.Name != "example.com" || info.Configured {
		t.Fatalf("original settings not shown: %+v %v", info, err)
	}
}

func TestPlausibleServiceInfoKeepsDisabledOriginalServer(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Imported", IPNS: "k51example", Imported: true}
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Imported","articles":[],"plausibleEnabled":false,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com:8443"}`)
	if err = s.Import(site, nil, raw); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.Server != "https://stats.example.com:8443" || info.Name != "example.com" || info.TrackingEnabled {
		t.Fatalf("disabled settings lost: %+v %v", info, err)
	}
	if err = lib.ConfigurePlausibleAt(site.ID, "example.com", "secret", "", ""); err == nil {
		t.Fatal("empty server accepted with credential")
	}
}

func TestDisablingPlausibleKeepsSelfHostedServerForFutureConfiguration(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigurePlausibleAt(site.ID, "example.com", "secret", "pa-Test123", "https://stats.example.com:8443"); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureService(site.ID, "plausible", "", ""); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.TrackingEnabled || info.Configured || info.Server != "https://stats.example.com:8443" || info.Name != "example.com" {
		t.Fatalf("lost disabled server: %+v %v", info, err)
	}
}

func TestDisablingImportedPlausibleKeepsItsServer(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Imported", IPNS: "k51example", Imported: true}
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Imported","articles":[],"plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com"}`)
	if err = s.Import(site, nil, raw); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureService(site.ID, "plausible", "", ""); err != nil {
		t.Fatal(err)
	}
	info, err := lib.ServiceInfo(site.ID, "plausible")
	if err != nil || info.TrackingEnabled || info.Server != "https://stats.example.com" || info.Name != "example.com" {
		t.Fatalf("lost imported server: %+v %v", info, err)
	}
}

func TestTemplateSettingsValidateAndPersist(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetTemplate(site.ID, "template-test", "Sepia"); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveTemplateSettings(site.ID, `{"backgroundColor":"bad"}`); err == nil {
		t.Fatal("invalid color accepted")
	}
	if _, err = lib.SaveTemplateSettings(site.ID, `{"unknown":"secret"}`); err == nil {
		t.Fatal("unknown setting accepted")
	}
	updated, err := lib.SaveTemplateSettings(site.ID, `{"backgroundColor":"#123456","showRef":"true"}`)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TemplateSettings["backgroundColor"] != "#123456" {
		t.Fatalf("wrong settings: %+v", updated)
	}
	settings, err := lib.TemplateSettings(site.ID)
	if err != nil || settings["backgroundColor"] != "#123456" || settings["linkColor"] == "" {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
}

func TestTemplateSettingsLoadsImportedValuesAndDefaults(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "site", Name: "Site", IPNS: "identity", Imported: true}
	if err = s.Import(site, nil, []byte(`{"templateName":"Sepia","articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.SetTemplate(site.ID, "template-test", "Sepia"); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "imports", site.ID)
	if err = os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(archive, "templateSettings.json"), []byte(`{"backgroundColor":"#112233","showRef":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := lib.TemplateSettings(site.ID)
	if err != nil || settings["backgroundColor"] != "#112233" || settings["showRef"] != "true" || settings["linkColor"] == "" {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
}

func TestPinnableConfigurationIsExplicitAndIsolatedBySite(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	first, err := lib.CreateSite("First", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.CreateSite("Second", "")
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := lib.PinnableConfigured(first.ID); err != nil || enabled {
		t.Fatalf("unexpected configuration: %v %v", enabled, err)
	}
	if err := lib.ConfigurePinnable(first.ID, "https://evil.test/pin/id"); err == nil {
		t.Fatal("untrusted endpoint accepted")
	}
	if err := lib.ConfigurePinnable(first.ID, "https://pinnable.xyz/pin/1234-abcd"); err != nil {
		t.Fatal(err)
	}
	if enabled, err := lib.PinnableConfigured(first.ID); err != nil || !enabled {
		t.Fatalf("not configured: %v %v", enabled, err)
	}
	if enabled, err := lib.PinnableConfigured(second.ID); err != nil || enabled {
		t.Fatalf("other site configured: %v %v", enabled, err)
	}
	if err := lib.ConfigurePinnable(first.ID, ""); err != nil {
		t.Fatal(err)
	}
	if enabled, err := lib.PinnableConfigured(first.ID); err != nil || enabled {
		t.Fatalf("not disabled: %v %v", enabled, err)
	}
}

func TestUnfollowRemovesSubscriptionAndQueuedRefreshCannotRestoreIt(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>News</title><item><guid>one</guid><title>Post</title></item></channel></rss>`))
	}))
	defer server.Close()
	lib := &Library{}
	if _, err := lib.Follow(server.URL); err != nil {
		t.Fatal(err)
	}
	if err := lib.Unfollow(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RefreshSubscription(server.URL); err == nil {
		t.Fatal("queued refresh restored a removed subscription")
	}
	if requests.Load() != 1 {
		t.Fatal("removed subscription caused another network request")
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := reopened.ListSubscriptions()
	if err != nil || len(subs) != 0 {
		t.Fatalf("subscription returned after restart: %+v %v", subs, err)
	}
}

func TestUnfollowWaitsForOngoingRefresh(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	var count atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 2 {
			close(started)
			<-release
		}
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>News</title></channel></rss>`))
	}))
	defer server.Close()
	lib := &Library{}
	if _, err := lib.Follow(server.URL); err != nil {
		t.Fatal(err)
	}
	refreshed := make(chan error, 1)
	go func() { _, err := lib.RefreshSubscription(server.URL); refreshed <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	removed := make(chan error, 1)
	go func() { removed <- lib.Unfollow(server.URL) }()
	select {
	case err := <-removed:
		close(release)
		t.Fatalf("removed during refresh: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	subs, err := lib.Subscriptions()
	if err != nil || len(subs) != 0 {
		t.Fatalf("removed subscription restored: %+v %v", subs, err)
	}
}

func TestSiteRemovalWaitsForPublicationAndPreventsLaterSnapshots(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	lib.publicMu.Lock()
	removed := make(chan error, 1)
	go func() { removed <- lib.RemoveSite(site.ID) }()
	select {
	case err := <-removed:
		lib.publicMu.Unlock()
		t.Fatalf("removed while publication was active: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	lib.publicMu.Unlock()
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.PublicationSnapshot(site.ID); err == nil {
		t.Fatal("removed site remains publishable")
	}
}

func TestPermanentDeletionRemovesOnlySelectedSiteData(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := lib.CreateSite("Keep", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveDraft(site.ID, "", article.ID, "Draft", "private"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"attachments", "draft-attachments", "imports"} {
		for _, id := range []string{site.ID, other.ID} {
			dir := filepath.Join(root, kind, id)
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = lib.ConfigurePinnable(site.ID, "https://pinnable.xyz/pin/site-123"); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"attachments", "draft-attachments", "imports"} {
		if _, err = os.Stat(filepath.Join(root, kind, site.ID)); !os.IsNotExist(err) {
			t.Fatalf("site data retained: %s %v", kind, err)
		}
		if _, err = os.Stat(filepath.Join(root, kind, other.ID, "file")); err != nil {
			t.Fatalf("other site damaged: %s %v", kind, err)
		}
	}
	if _, err = os.Stat(filepath.Join(root, "integrations", "pinnable", site.ID+".dat")); !os.IsNotExist(err) {
		t.Fatalf("credential retained: %v", err)
	}
	removed, err := lib.RemovedSites()
	if err != nil || len(removed) != 0 {
		t.Fatalf("site retained: %+v %v", removed, err)
	}
}

func TestInterruptedPermanentDeletionRemainsVisibleAndRetryable(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "attachments"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "attachments", site.ID)
	if err = os.WriteFile(path, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err == nil {
		t.Fatal("unexpected cleanup success")
	}
	removed, err := lib.RemovedSites()
	if err != nil || len(removed) != 1 || !removed[0].DeletionPending {
		t.Fatalf("deletion state lost: %+v %v", removed, err)
	}
	if _, err = lib.RestoreSite(site.ID); err == nil {
		t.Fatal("partially deleted site restored")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPermanentDeletionRemovesMatchingKeyAndPreservesOtherKuboKeys(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := lib.CreateSite("Keep", "")
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	lib.node = node
	identity, err := node.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := node.EnsureKey(other.ID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if key, err := node.LookupKey(site.ID); err != nil || key != "" {
		t.Fatalf("site key remains: %s %v", key, err)
	}
	if key, err := node.LookupKey(other.ID); err != nil || key != otherKey {
		t.Fatalf("other key changed: %s %v", key, err)
	}
}

func TestPermanentDeletionRefusesMismatchedKeyBeforeDeletingFiles(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	lib.node = node
	actual, err := node.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	unexpected, err := node.EnsureKey("other")
	if err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, unexpected); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "attachments", site.ID)
	if err = os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err == nil {
		t.Fatal("mismatched identity deleted")
	}
	removed, err := lib.RemovedSites()
	if err != nil || len(removed) != 1 || removed[0].DeletionPending {
		t.Fatalf("unexpected deletion state: %+v %v", removed, err)
	}
	if _, err = os.Stat(filepath.Join(path, "keep")); err != nil {
		t.Fatal("file deleted", err)
	}
	if key, err := node.LookupKey(site.ID); err != nil || key != actual {
		t.Fatalf("key changed: %s %v", key, err)
	}
}

func TestPermanentDeletionRejectsRedirectedRealKuboKeystore(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows junction required")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	outside := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	local := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = local.Init(); err != nil {
		t.Fatal(err)
	}
	remote := ipfs.New(exe, filepath.Join(outside, "ipfs"))
	if err = remote.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := remote.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	keystore := filepath.Join(root, "ipfs", "keystore")
	if err = os.Remove(keystore); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", keystore, filepath.Join(outside, "ipfs", "keystore")).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s %v", out, err)
	}
	t.Cleanup(func() { _ = os.Remove(keystore) })
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err == nil {
		t.Fatal("redirected keystore accepted")
	}
	if key, err := remote.LookupKey(site.ID); err != nil || key != identity {
		t.Fatalf("external key changed: %s %v", key, err)
	}
	removed, err := lib.RemovedSites()
	if err != nil || len(removed) != 1 || removed[0].DeletionPending {
		t.Fatalf("unsafe deletion started: %+v %v", removed, err)
	}
}

func TestPermanentDeletionRejectsForeignKuboAPI(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	foreignRoot := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	local := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = local.Init(); err != nil {
		t.Fatal(err)
	}
	foreign := ipfs.New(exe, filepath.Join(foreignRoot, "ipfs"))
	if err = foreign.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := foreign.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(foreignRoot, "key.bin")
	if err = foreign.ExportKey(site.ID, keyfile); err != nil {
		t.Fatal(err)
	}
	if _, err = local.ImportKey(site.ID, keyfile); err != nil {
		t.Fatal(err)
	}
	if err = foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	api, err := os.ReadFile(filepath.Join(foreignRoot, "ipfs", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "ipfs", "api"), api, 0600); err != nil {
		t.Fatal(err)
	}
	lib.node = local
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err == nil {
		t.Fatal("foreign API accepted")
	}
	if key, err := foreign.LookupKey(site.ID); err != nil || key != identity {
		t.Fatalf("foreign key changed: %s %v", key, err)
	}
	if err = os.Remove(filepath.Join(root, "ipfs", "api")); err != nil {
		t.Fatal(err)
	}
	if key, err := local.LookupKey(site.ID); err != nil || key != identity {
		t.Fatalf("local key changed: %s %v", key, err)
	}
}

func TestPermanentDeletionWorksWithAppOwnedKuboDaemon(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := node.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = node.Start(true); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	lib.node = node
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if key, err := node.LookupKey(site.ID); err != nil || key != "" {
		t.Fatalf("key retained: %s %v", key, err)
	}
}

func TestPermanentDeletionRejectsTamperedAppOwnedAPI(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	foreignRoot := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	local := ipfs.New(exe, filepath.Join(root, "ipfs"))
	foreign := ipfs.New(exe, filepath.Join(foreignRoot, "ipfs"))
	if err = local.Init(); err != nil {
		t.Fatal(err)
	}
	if err = foreign.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := foreign.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(foreignRoot, "key.bin")
	if err = foreign.ExportKey(site.ID, keyfile); err != nil {
		t.Fatal(err)
	}
	if _, err = local.ImportKey(site.ID, keyfile); err != nil {
		t.Fatal(err)
	}
	if err = foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if err = local.Start(true); err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	api, err := os.ReadFile(filepath.Join(foreignRoot, "ipfs", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "ipfs", "api"), api, 0600); err != nil {
		t.Fatal(err)
	}
	lib.node = local
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = lib.PermanentlyDeleteSite(site.ID); err == nil {
		t.Fatal("tampered API accepted")
	}
	if key, err := foreign.LookupKey(site.ID); err != nil || key != identity {
		t.Fatalf("foreign key changed: %s %v", key, err)
	}
	if key, err := local.LookupKey(site.ID); err != nil || key != identity {
		t.Fatalf("local key changed: %s %v", key, err)
	}
}

func TestStartupRetriesPendingSiteDeletion(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginSiteDeletion(site.ID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "attachments", site.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	lib.resumeSiteDeletions()
	removed, err := lib.RemovedSites()
	if err != nil || len(removed) != 0 {
		t.Fatalf("cleanup not resumed: %+v %v", removed, err)
	}
}

func TestLibraryPublishesAndPersistsIdentity(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	library := &Library{}
	t.Cleanup(library.closeNode)
	site, err := library.CreateSite("Planet Windows integration test", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = library.SaveArticle(site.ID, "", "Synthetic post", "Public test content"); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].IPNS != result.IPNS || sites[0].LastPublishedCID != result.CID || sites[0].LastPublished == nil {
		t.Fatalf("result=%#v sites=%#v", result, sites)
	}
	t.Logf("IPNS=%s CID=%s", result.IPNS, result.CID)
}

func TestLibraryPublishesImportedSiteWithAttachments(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "app"))
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	name, err := source.EnsureKey(id)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "synthetic.planet")
	if err = os.MkdirAll(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	put := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(input, path), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("index.html", "Synthetic imported site")
	put(filepath.Join(post, "index.html"), "Imported article with attachment")
	put(filepath.Join(post, "photo.txt"), "synthetic attachment")
	public := fmt.Sprintf(`{"id":%q,"link":%q,"title":"Imported post","content":"Synthetic content","created":10,"attachments":["photo.txt"]}`, post, "/"+post+"/")
	put(filepath.Join(post, "article.json"), public)
	metadata := fmt.Sprintf(`{"id":%q,"name":"Imported","about":"","ipns":%q,"created":10,"updated":20,"templateName":"Plain","filebaseAPIToken":"SECRET_TEST_TOKEN","articles":[%s]}`, id, name, public)
	put("planet.json", metadata)
	if err = source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	library := &Library{}
	t.Cleanup(library.closeNode)
	s, err := library.open()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = library.ensureNode(false)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := migration.New(filepath.Join(root, "app"), exe, s, library.node).Import(input)
	if err != nil {
		t.Fatal(err)
	}
	templateRoot := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if templateRoot == "" {
		t.Fatal("Plain template required")
	}
	posts, err := s.ListArticles(imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(filepath.Join(root, "app"), templateRoot, imported, posts, s); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveArticle(imported.ID, post, "Updated imported post", "Updated synthetic content"); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.IPNS != name {
		t.Fatalf("identity changed: %s", result.IPNS)
	}
	readerRepo := filepath.Join(root, "reader")
	reader := ipfs.New(exe, readerRepo)
	if err = reader.Init(); err != nil {
		t.Fatal(err)
	}
	if err = reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Env = append(os.Environ(), "IPFS_PATH="+readerRepo)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s (%v)", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := run("name", "resolve", "--nocache", "/ipns/"+name); got != "/ipfs/"+result.CID {
		t.Fatalf("resolved %s", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/"+post+"/photo.txt"); got != "synthetic attachment" {
		t.Fatalf("attachment=%q", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/"+post+"/index.html"); !strings.Contains(got, "Updated synthetic content") {
		t.Fatalf("stale article page: %s", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/planet.json"); strings.Contains(got, "SECRET_TEST_TOKEN") || strings.Contains(got, "filebaseAPIToken") {
		t.Fatalf("private metadata published: %s", got)
	}
	t.Logf("Synthetic imported site: IPNS=%s CID=%s", result.IPNS, result.CID)
}

func TestLibraryPublishesWindowsAttachment(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "app"))
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	library := &Library{}
	t.Cleanup(library.closeNode)
	site, err := library.CreateSite("Synthetic attachments", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := library.SaveArticle(site.ID, "", "Post", "![photo](photo.png)")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "photo.png")
	if err = os.WriteFile(file, []byte("synthetic image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := library.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Add(filepath.Join(root, "app"), s, site.ID, post.ID, file); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "reader")
	reader := ipfs.New(exe, repo)
	if err = reader.Init(); err != nil {
		t.Fatal(err)
	}
	if err = reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "cat", "/ipfs/"+result.CID+"/"+post.ID+"/photo.png")
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repo)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "synthetic image bytes" {
		t.Fatalf("attachment=%q err=%v", out, err)
	}
}

func TestPreviewArticleUsesOnlyItsRegisteredImages(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "photo.png")
	os.WriteFile(file, []byte("\x89PNG\r\n\x1a\nimage"), 0600)
	s, err := lib.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Add(root, s, site.ID, post.ID, file); err != nil {
		t.Fatal(err)
	}
	rendered, err := lib.PreviewArticle(site.ID, post.ID, "![image](photo.png)")
	if err != nil || !strings.Contains(rendered, "data:image/png;base64,") {
		t.Fatalf("preview=%s err=%v", rendered, err)
	}
	if _, err = lib.PreviewArticle("other", post.ID, "![image](photo.png)"); err == nil {
		t.Fatal("cross-site preview accepted")
	}
}

func TestUnsavedPreviewDoesNotPermitEmbeddedDataImages(t *testing.T) {
	html, err := (&Library{}).Preview("![inline](data:image/png;base64,AAAA)")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "base64,AAAA") {
		t.Fatalf("untrusted data image retained: %s", html)
	}
}

func TestAudioURLStreamsOnlySelectedRegisteredArticle(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	t.Cleanup(lib.closeAudio)
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "audio.mp3")
	os.WriteFile(file, []byte("0123456789"), 0600)
	s, _ := lib.open()
	post, err = attachments.Add(root, s, site.ID, post.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	address, err := lib.AudioURL(site.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", address, nil)
	req.Header.Set("Range", "bytes=2-5")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || string(data) != "2345" {
		t.Fatalf("status=%d data=%q", response.StatusCode, data)
	}
	if _, err = attachments.Remove(root, s, site.ID, post.ID, "audio.mp3"); err != nil {
		t.Fatal(err)
	}
	response, err = http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("deleted file remains accessible: %d", response.StatusCode)
	}
}

func TestFollowAndPinPlanetUsingOfflineNode(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	node := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := node.EnsureKey("test-site"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "nested", "site", "post"), 0700)
	os.WriteFile(filepath.Join(dir, "nested", "site", "post", "photo.png"), []byte("\x89PNG\r\n\x1a\nimage"), 0600)
	os.WriteFile(filepath.Join(dir, "nested", "site", "post", "sound.mp3"), []byte("synthetic audio"), 0600)
	os.WriteFile(filepath.Join(dir, "nested", "site", "planet.json"), []byte(`{"name":"Offline Planet","about":"","articles":[{"id":"post","link":"/post/","title":"Test","content":"![photo](photo.png)","created":10,"audioFilename":"sound.mp3","attachments":["photo.png","sound.mp3"]}]}`), 0600)
	cid, err := node.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	publish := exec.Command(exe, "--offline", "name", "publish", "--allow-offline", "--quieter", "--key=test-site", "/ipfs/"+cid+"/nested/site")
	publish.Env = append(os.Environ(), "IPFS_PATH="+filepath.Join(root, "ipfs"))
	published, err := publish.CombinedOutput()
	if err != nil {
		t.Fatalf("publish: %s %v", published, err)
	}
	name := strings.TrimSpace(string(published))
	if err = node.Start(true); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	lib := &Library{node: node}
	sub, err := lib.Follow("planet://" + name)
	if err != nil {
		t.Fatal(err)
	}
	if sub.CID != cid || sub.Feed.Title != "Offline Planet" || len(sub.Feed.Items) != 1 || sub.Feed.Items[0].ContentPath != "nested/site" {
		t.Fatalf("sub=%+v", sub)
	}
	removePin := exec.Command(exe, "pin", "rm", cid)
	removePin.Env = append(os.Environ(), "IPFS_PATH="+filepath.Join(root, "ipfs"))
	if out, e := removePin.CombinedOutput(); e != nil {
		t.Fatalf("remove initial pin: %s %v", out, e)
	}
	if err = lib.PinSubscription(sub.URL); err != nil {
		t.Fatal(err)
	}
	gc := exec.Command(exe, "repo", "gc")
	gc.Env = append(os.Environ(), "IPFS_PATH="+filepath.Join(root, "ipfs"))
	if out, e := gc.CombinedOutput(); e != nil {
		t.Fatalf("garbage collection: %s %v", out, e)
	}
	if err = node.Close(); err != nil {
		t.Fatal(err)
	}
	node = ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = node.Start(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	lib = &Library{node: node}
	subs, err := lib.Subscriptions()
	if err != nil || subs[0].PinnedCID != cid {
		t.Fatalf("subs=%+v err=%v", subs, err)
	}
	defer lib.closeAudio()
	address, err := lib.SubscriptionAudioURL(sub.URL, subs[0].Feed.Items[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(audio) != "synthetic audio" {
		t.Fatalf("offline audio=%q err=%v", audio, err)
	}
	html, err := lib.SubscriptionPreview(sub.URL, subs[0].Feed.Items[0].Key)
	if err != nil || !strings.Contains(html, "data:image/png;base64,") {
		t.Fatalf("offline image preview=%s err=%v", html, err)
	}
	if err = lib.StarSubscription(sub.URL, subs[0].Feed.Items[0].Key, true); err != nil {
		t.Fatal(err)
	}
	subs, _ = lib.Subscriptions()
	if !subs[0].Feed.Items[0].Starred {
		t.Fatal("star missing")
	}
}

func TestFeedRefreshFailureKeepsCachedAndStarredArticles(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>News</title><item><guid>one</guid><title>Post</title><link>https://example.test/post</link></item></channel></rss>`))
	}))
	defer server.Close()
	lib := &Library{}
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.StarSubscription(sub.URL, sub.Feed.Items[0].Key, true); err != nil {
		t.Fatal(err)
	}
	fail = true
	if _, err = lib.Follow(server.URL); err == nil {
		t.Fatal("refresh should fail")
	}
	subs, _ := lib.Subscriptions()
	if subs[0].LastError == "" || !subs[0].Feed.Items[0].Starred {
		t.Fatalf("cached subscription=%+v", subs[0])
	}
	fail = false
	if _, err = lib.Follow(server.URL); err != nil {
		t.Fatal(err)
	}
	subs, _ = lib.Subscriptions()
	if subs[0].LastError != "" || !subs[0].Feed.Items[0].Starred {
		t.Fatalf("refreshed subscription=%+v", subs[0])
	}
}

func TestFeedMediaPinSurvivesRestartAndNetworkLoss(t *testing.T) {
	for _, format := range []string{"rss", "atom", "json"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			mygo.App.SetPath(mygo.PathUserData, root)
			t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/feed":
					if format == "rss" {
						fmt.Fprintf(w, `<rss version="2.0"><channel><title>Feed</title><description>Feed</description><link>%s</link><item><guid>one</guid><title>Post</title><link>%s/post</link><description><![CDATA[<img src="/image.png"><video src="/video.mp4"></video>]]></description><enclosure url="%s/audio.mp3" type="audio/mpeg" length="5"/></item></channel></rss>`, server.URL, server.URL, server.URL)
						return
					}
					if format == "atom" {
						fmt.Fprintf(w, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Feed</title><id>feed</id><updated>2026-01-01T00:00:00Z</updated><entry><id>one</id><title>Post</title><updated>2026-01-01T00:00:00Z</updated><link href="%s/post"/><link rel="enclosure" href="%s/audio.mp3" type="audio/mpeg"/><content type="html">&lt;img src="/image.png"&gt;&lt;video src="/video.mp4"&gt;&lt;/video&gt;</content></entry></feed>`, server.URL, server.URL)
						return
					}
					w.Header().Set("Content-Type", "application/feed+json")
					fmt.Fprintf(w, `{"version":"https://jsonfeed.org/version/1.1","title":"Feed","items":[{"id":"one","url":%q,"title":"Post","content_html":%q,"attachments":[{"url":%q,"mime_type":"audio/mpeg"},{"url":%q,"mime_type":"video/mp4"}]}]}`, server.URL+"/post", `<img src="/image.png"><video src="/video.mp4"></video>`, server.URL+"/audio.mp3", server.URL+"/video.mp4")
				case "/image.png":
					w.Header().Set("Content-Type", "image/png")
					w.Write([]byte("\x89PNG\r\n\x1a\nimage"))
				case "/audio.mp3":
					w.Header().Set("Content-Type", "audio/mpeg")
					w.Write([]byte("audio"))
				case "/video.mp4":
					w.Header().Set("Content-Type", "video/mp4")
					w.Write([]byte("video"))
				default:
					http.NotFound(w, r)
				}
			}))
			lib := &Library{}
			sub, err := lib.Follow(server.URL + "/feed")
			if err != nil {
				server.Close()
				t.Fatal(err)
			}
			if err = lib.PinSubscription(sub.URL); err != nil {
				server.Close()
				t.Fatal(err)
			}
			server.Close()
			lib.closeAudio()
			lib = &Library{}
			defer lib.closeAudio()
			subs, err := lib.Subscriptions()
			if err != nil || !subs[0].OfflinePinned {
				t.Fatalf("subs=%+v err=%v", subs, err)
			}
			item := subs[0].Feed.Items[0]
			html, err := lib.SubscriptionPreview(sub.URL, item.Key)
			if err != nil || !strings.Contains(html, "data:image/png;base64,") {
				t.Fatalf("preview=%s err=%v", html, err)
			}
			address, err := lib.SubscriptionAudioURL(sub.URL, item.Key)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.Get(address)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(b) != "audio" {
				t.Fatalf("audio=%q", b)
			}
			address, err = lib.SubscriptionMediaURL(sub.URL, item.Key, item.VideoURL)
			if err != nil {
				t.Fatal(err)
			}
			resp, err = http.Get(address)
			if err != nil {
				t.Fatal(err)
			}
			b, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(b) != "video" {
				t.Fatalf("video=%q", b)
			}
		})
	}
}

func TestPinnedFeedRefreshFailureKeepsOfflineMedia(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	fail := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			if fail {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("original"))
			return
		}
		fmt.Fprintf(w, `{"version":"https://jsonfeed.org/version/1.1","title":"Feed","items":[{"id":"one","title":"Episode","content_text":"Text","attachments":[{"url":%q,"mime_type":"audio/mpeg"}]}]}`, server.URL+"/audio")
	}))
	defer server.Close()
	lib := &Library{}
	defer lib.closeAudio()
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.PinSubscription(sub.URL); err != nil {
		t.Fatal(err)
	}
	if err = lib.StarSubscription(sub.URL, sub.Feed.Items[0].Key, true); err != nil {
		t.Fatal(err)
	}
	fail = true
	if _, err = lib.Follow(sub.URL); err == nil {
		t.Fatal("refresh accepted missing media")
	}
	subs, err := lib.Subscriptions()
	if err != nil || !subs[0].OfflinePinned || !subs[0].Feed.Items[0].Starred || subs[0].LastError == "" {
		t.Fatalf("subscription=%+v err=%v", subs, err)
	}
	media, err := lib.SubscriptionAudioURL(sub.URL, subs[0].Feed.Items[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(media)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(data) != "original" {
		t.Fatalf("media=%q", data)
	}
}

func TestOfflineCleanupPreservesPlayingAndReferencedMedia(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	value := "old"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte(value))
			return
		}
		fmt.Fprintf(w, `{"version":"https://jsonfeed.org/version/1.1","title":"Feed","items":[{"id":"one","title":"Episode","content_text":"Text","attachments":[{"url":%q,"mime_type":"audio/mpeg"}]}]}`, server.URL+"/audio")
	}))
	defer server.Close()
	lib := &Library{}
	defer lib.closeAudio()
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.PinSubscription(sub.URL); err != nil {
		t.Fatal(err)
	}
	audio, err := lib.SubscriptionAudioURL(sub.URL, sub.Feed.Items[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	value = "new content"
	if _, err = lib.Follow(sub.URL); err != nil {
		t.Fatal(err)
	}
	usage, err := lib.CleanupOfflineMedia()
	if err != nil {
		t.Fatal(err)
	}
	if usage.ReclaimedBytes != 0 {
		t.Fatalf("deleted active file: %+v", usage)
	}
	resp, err := http.Get(audio)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "old" {
		t.Fatalf("playing data %q", data)
	}
	lib.StopAudio(audio)
	usage, err = lib.CleanupOfflineMedia()
	if err != nil {
		t.Fatal(err)
	}
	if usage.ReclaimedBytes != 3 {
		t.Fatalf("cleanup after stop: %+v", usage)
	}
	subs, err := lib.Subscriptions()
	if err != nil {
		t.Fatal(err)
	}
	next, err := lib.SubscriptionAudioURL(sub.URL, subs[0].Feed.Items[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(next)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "new content" {
		t.Fatalf("current data %q", data)
	}
}

func TestCleanupWaitsForInProgressFeedRefresh(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	reached := make(chan struct{})
	release := make(chan struct{})
	var count atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio" {
			if count.Add(1) == 2 {
				close(reached)
				<-release
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("media"))
			return
		}
		fmt.Fprintf(w, `{"version":"https://jsonfeed.org/version/1.1","title":"Feed","items":[{"id":"one","content_text":"Text","attachments":[{"url":%q,"mime_type":"audio/mpeg"}]}]}`, server.URL+"/audio")
	}))
	defer server.Close()
	lib := &Library{}
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.PinSubscription(sub.URL); err != nil {
		t.Fatal(err)
	}
	refresh := make(chan error, 1)
	go func() { _, err := lib.Follow(sub.URL); refresh <- err }()
	<-reached
	started := make(chan struct{})
	cleaned := make(chan error, 1)
	go func() { close(started); _, err := lib.CleanupOfflineMedia(); cleaned <- err }()
	<-started
	select {
	case err := <-cleaned:
		close(release)
		t.Fatalf("cleanup raced with refresh: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err = <-refresh; err != nil {
		t.Fatal(err)
	}
	if err = <-cleaned; err != nil {
		t.Fatal(err)
	}
	subs, err := lib.Subscriptions()
	if err != nil {
		t.Fatal(err)
	}
	asset := subs[0].Feed.Items[0].OfflineMedia[server.URL+"/audio"]
	f, err := offline.Open(root, asset)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
