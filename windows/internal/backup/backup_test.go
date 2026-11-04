package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"windows/internal/store"
)

const fixture = `{"id":"11111111-1111-4111-8111-111111111111","name":"站点","about":"About","ipns":"k51example","created":10,"updated":12,"templateName":"Plain","unknownField":{"value":42},"articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Title","content":"Body","created":11,"starType":0,"unknownArticle":"preserve"}]}`

func TestMergePreservesOriginalSecretAndExportsTrackingOverride(t *testing.T) {
	raw := []byte(`{"plausibleEnabled":true,"plausibleDomain":"new.example","plausibleAPIKey":"private","articles":[]}`)
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "new.example"}}
	out, err := Merge(raw, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if json.Unmarshal(out, &data) != nil || data["plausibleDomain"] != "new.example" || data["plausibleAPIKey"] != nil || !strings.Contains(string(out), "private") {
		t.Fatalf("unexpected backup %s", out)
	}
}

func TestMergeDoesNotBindOriginalPlausibleKeyToNewServer(t *testing.T) {
	raw := []byte(`{"plausibleEnabled":true,"plausibleDomain":"old.example","plausibleAPIServer":"old.example","plausibleAPIKey":"OLD-SECRET","articles":[]}`)
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "new.example", Server: "https://new.example"}}
	out, err := Merge(raw, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["plausibleAPIKey"]) == `"OLD-SECRET"` {
		t.Fatal("original token was bound to new server")
	}
	if !strings.Contains(string(m["windowsPreviousPlausibleConfig"]), "OLD-SECRET") {
		t.Fatal("original private credential was lost")
	}
}

func TestMergeDoesNotReactivateOriginalPlausibleKeyAfterWindowsOverride(t *testing.T) {
	raw := []byte(`{"plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com","plausibleAPIKey":"EXPIRED-SECRET","articles":[]}`)
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", Server: "https://stats.example.com"}}
	out, err := Merge(raw, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(out, &m) != nil {
		t.Fatal("invalid JSON")
	}
	if string(m["plausibleAPIKey"]) == `"EXPIRED-SECRET"` || !strings.Contains(string(m["windowsPreviousPlausibleConfig"]), "EXPIRED-SECRET") {
		t.Fatal("old key was reactivated or lost")
	}
}

func TestUnchangedExportRequiresSanitizingPlausibleKeyAfterOverride(t *testing.T) {
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Site","about":"","templateName":"Plain","plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com","plausibleAPIKey":"OLD-KEY","windowsPlausibleScriptID":"pa-Test123","articles":[]}`)
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", Server: "https://stats.example.com", ScriptID: "pa-Test123"}}
	if err := CheckUnchanged(raw, site, nil); err == nil {
		t.Fatal("export would bypass private key isolation")
	}
}

func TestDisabledPlausibleBackupRetainsServerWithoutActivatingKey(t *testing.T) {
	raw := []byte(`{"plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com","plausibleAPIKey":"OLD-KEY","articles":[]}`)
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: false, Domain: "example.com", Server: "https://stats.example.com"}}
	out, err := Merge(raw, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	enabled, domain, server, err := PlausibleStoredSettings(out)
	if err != nil || enabled || domain != "example.com" || server != "https://stats.example.com" {
		t.Fatalf("disabled settings lost: %s %v", out, err)
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(out, &m)
	if len(m["plausibleAPIKey"]) > 0 || !strings.Contains(string(m["windowsPreviousPlausibleConfig"]), "OLD-KEY") {
		t.Fatal("key reactivated or lost")
	}
}

func TestWindowsBackupRetainsSiteSpecificScriptOnReimport(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", IPNS: "k51example", Created: time.Now(), PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", ScriptID: "pa-Test123", Server: "https://stats.example.com:8443"}}
	metadata, err := Merge(nil, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "site.planet")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "planet.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "planet.key"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Site.PlausibleOverride == nil || parsed.Site.PlausibleOverride.ScriptID != "pa-Test123" || parsed.Site.PlausibleOverride.Server != "https://stats.example.com:8443" {
		t.Fatalf("script ID lost: %+v", parsed.Site)
	}
	site.PlausibleOverride = &store.PlausibleTracking{Enabled: false}
	metadata, err = Merge(metadata, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), "windowsPlausibleScriptID") {
		t.Fatal("disabled script ID retained")
	}
}

func TestReadPreservesUnsupportedOriginalPlausibleMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site.planet")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	metadata := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","ipns":"k51example","created":10,"templateName":"Plain","articles":[],"plausibleEnabled":"unknown"}`
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := Read(dir)
	if err != nil || !strings.Contains(string(parsed.Raw), `"unknown"`) {
		t.Fatalf("original metadata not preserved: %v", err)
	}
}

func TestMergeExportsNewArticleNumbersAndNextCounter(t *testing.T) {
	var site store.Site
	if err := json.Unmarshal([]byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Site","nextArticleNumber":12}`), &site); err != nil {
		t.Fatal(err)
	}
	article := store.Article{ID: "33333333-3333-4333-8333-333333333333", SiteID: site.ID, Title: "New", ArticleNumber: intPointer(11)}
	out, err := Merge([]byte(`{"nextArticleNumber":10,"articles":[]}`), site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Next     int `json:"nextArticleNumber"`
		Articles []struct {
			Number int `json:"articleNumber"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Next != 12 || len(decoded.Articles) != 1 || decoded.Articles[0].Number != 11 {
		t.Fatalf("backup numbers: %s", out)
	}
}

func TestMergeAdvancesStaleCounterPastSavedArticle(t *testing.T) {
	var site store.Site
	if err := json.Unmarshal([]byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Site","nextArticleNumber":2}`), &site); err != nil {
		t.Fatal(err)
	}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Post", ArticleNumber: intPointer(2)}
	out, err := Merge(nil, site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Next int `json:"nextArticleNumber"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Next != 3 {
		t.Fatalf("counter collides with article: %s", out)
	}
}

func intPointer(v int) *int { return &v }

func TestMergeUsesCurrentPublicationState(t *testing.T) {
	published := time.Unix(978307300, 0).UTC()
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "站点", About: "About", IPNS: "k51example", Created: time.Unix(978307210, 0), LastPublishedCID: "new-cid", LastPublished: &published}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Title", Content: "Body", Created: time.Unix(978307211, 0), Modified: time.Unix(978307211, 0)}
	input := strings.Replace(fixture, `"templateName":"Plain"`, `"templateName":"Plain","lastPublished":12,"lastPublishedCID":"old-cid"`, 1)
	b, err := Merge([]byte(input), site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["lastPublishedCID"] != "new-cid" || out["lastPublished"] != float64(100) {
		t.Fatalf("stale publication metadata: %s", b)
	}
}

func TestDecodeAndMergePreservesUnknownFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sample.planet")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "22222222-2222-4222-8222-222222222222"), 0700); err != nil {
		t.Fatal(err)
	}
	parsed, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Site.Name != "站点" || parsed.Site.Created.Unix() != 978307210 || len(parsed.Articles) != 1 {
		t.Fatalf("parsed = %#v", parsed)
	}
	parsed.Articles[0].Title = "Changed"
	encoded, err := Merge(parsed.Raw, parsed.Site, parsed.Articles)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["unknownField"].(map[string]any)["value"] != float64(42) {
		t.Fatal("site metadata lost")
	}
	article := result["articles"].([]any)[0].(map[string]any)
	if article["unknownArticle"] != "preserve" || article["title"] != "Changed" {
		t.Fatalf("article=%#v", article)
	}
}

func TestRejectsUnsafeBackupPaths(t *testing.T) {
	for _, name := range []string{"..", "CON", "name.", "bad:name", "dir/file", "NUL.txt"} {
		if SafeName(name) {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestRequiresKeyAndArticleDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sample.planet")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("missing article directory accepted")
	}
}

func TestCopyTreeKeepsAssetsButNotPrivateKey(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "assets", "style.css"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "planet.key"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "assets", "style.css"))
	if err != nil || string(b) != "ok" {
		t.Fatalf("asset=%q err=%v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("private key was copied: %v", err)
	}
}

func TestSimplePlainRebuildRejectsUnsupportedMetadata(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", About: "About"}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Updated", Content: "Text"}
	valid := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","about":"About","templateName":"Plain","articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Before","content":"Text","created":1,"starType":0}]}`
	if err := CheckSimplePlain([]byte(valid), site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	unsafe := strings.Replace(valid, `"templateName":"Plain"`, `"templateName":"Plain","customCodeHeadEnabled":true`, 1)
	if err := CheckSimplePlain([]byte(unsafe), site, []store.Article{article}); err == nil {
		t.Fatal("custom code accepted")
	}
	unknown := strings.Replace(valid, `"templateName":"Plain"`, `"templateName":"Plain","unknownFlag":false`, 1)
	if err := CheckSimplePlain([]byte(unknown), site, []store.Article{article}); err == nil {
		t.Fatal("unknown field accepted")
	}
	article.Content = "<iframe>unsafe</iframe>"
	if err := CheckSimplePlain([]byte(valid), site, []store.Article{article}); err == nil {
		t.Fatal("raw HTML was accepted")
	}
}

func TestMacPrewarmSettingCanBePreservedDuringRebuild(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site"}
	post := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Updated", Content: "Body"}
	base := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","ipns":"key","templateName":"Plain","prewarmNewPost":VALUE,"articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Before","content":"Body","created":1}]}`
	for _, tc := range []struct {
		value   string
		allowed bool
	}{
		{"true", true}, {"false", true}, {"null", true}, {`"true"`, false}, {"0", false}, {"[]", false}, {"{}", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			raw := []byte(strings.Replace(base, "VALUE", tc.value, 1))
			err := CheckSimplePlain(raw, site, []store.Article{post})
			if (err == nil) != tc.allowed {
				t.Fatalf("unexpected result: %v", err)
			}
			if !tc.allowed {
				return
			}
			merged, e := Merge(raw, site, []store.Article{post})
			if e != nil {
				t.Fatal(e)
			}
			var fields map[string]json.RawMessage
			if e = json.Unmarshal(merged, &fields); e != nil {
				t.Fatal(e)
			}
			if string(fields["prewarmNewPost"]) != tc.value {
				t.Fatalf("setting changed: %s", merged)
			}
		})
	}
}

func TestDisabledMacCustomCodeIsPreservedButEnabledOrMalformedCodeIsRejected(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site"}
	post := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Changed", Content: "Body"}
	base := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","ipns":"key","templateName":"Plain",FIELDS "articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Before","content":"Body","created":1}]}`
	for _, location := range []string{"Head", "BodyStart", "BodyEnd"} {
		for _, tc := range []struct {
			name, fields string
			allowed      bool
		}{
			{"disabled", `"customCode` + location + `Enabled":false,"customCode` + location + `":"<script>PRIVATE</script>",`, true},
			{"nil-enabled", `"customCode` + location + `Enabled":null,"customCode` + location + `":"PRIVATE",`, true},
			{"absent-enabled", `"customCode` + location + `":"PRIVATE",`, true},
			{"enabled", `"customCode` + location + `Enabled":true,"customCode` + location + `":"PRIVATE",`, false},
			{"enabled-empty", `"customCode` + location + `Enabled":true,"customCode` + location + `":"",`, false},
			{"numeric-flag", `"customCode` + location + `Enabled":0,"customCode` + location + `":"PRIVATE",`, false},
			{"invalid-code", `"customCode` + location + `Enabled":false,"customCode` + location + `":[],`, false},
		} {
			t.Run(location+"/"+tc.name, func(t *testing.T) {
				raw := []byte(strings.Replace(base, "FIELDS", tc.fields, 1))
				err := CheckSimplePlain(raw, site, []store.Article{post})
				if (err == nil) != tc.allowed {
					t.Fatalf("unexpected result: %v", err)
				}
				if !tc.allowed {
					return
				}
				merged, e := Merge(raw, site, []store.Article{post})
				if e != nil {
					t.Fatal(e)
				}
				var before, after map[string]json.RawMessage
				if e = json.Unmarshal(raw, &before); e != nil {
					t.Fatal(e)
				}
				if e = json.Unmarshal(merged, &after); e != nil {
					t.Fatal(e)
				}
				for _, key := range []string{"customCode" + location, "customCode" + location + "Enabled"} {
					b, beforeExists := before[key]
					a, afterExists := after[key]
					var beforeValue, afterValue any
					if beforeExists {
						if e = json.Unmarshal(b, &beforeValue); e != nil {
							t.Fatal(e)
						}
					}
					if afterExists {
						if e = json.Unmarshal(a, &afterValue); e != nil {
							t.Fatal(e)
						}
					}
					if beforeExists != afterExists || !reflect.DeepEqual(beforeValue, afterValue) {
						t.Fatalf("setting %s changed: %s", key, merged)
					}
				}
			})
		}
	}
}

func TestOverlayReplacesGeneratedFilesAndRetainsOtherFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, dir := range []string{src, dst} {
		if err := os.Mkdir(filepath.Join(dir, "article"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "article", "index.html"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "article", "index.html"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "article", "photo.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := OverlayTree(src, dst); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dst, "article", "index.html"))
	if err != nil || string(page) != "new" {
		t.Fatalf("page=%q err=%v", page, err)
	}
	asset, err := os.ReadFile(filepath.Join(dst, "article", "photo.txt"))
	if err != nil || string(asset) != "preserve" {
		t.Fatalf("asset=%q err=%v", asset, err)
	}
}

func TestMergeIncludesNewArticleAttachments(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", Created: time.Now()}
	post := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Post", Created: time.Now(), Modified: time.Now(), Attachments: []string{"photo.png"}}
	b, err := Merge(nil, site, []store.Article{post})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"attachments"`) || !strings.Contains(string(b), `"photo.png"`) {
		t.Fatalf("backup missing attachment metadata: %s", b)
	}
}

func TestImportedEditsMergeAndRebuild(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", About: "", TemplateName: "Grid", Created: time.Now()}
	original := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","about":"","templateName":"Plain","articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Old","content":"Body","created":1,"starType":0,"attachments":["old.png"]}]}`
	post := store.Article{ID: "33333333-3333-4333-8333-333333333333", SiteID: site.ID, Title: "New", Content: "Body", Created: time.Now(), Modified: time.Now(), Attachments: []string{"new.png"}}
	if err := CheckSimplePlain([]byte(original), site, []store.Article{post}); err != nil {
		t.Fatal(err)
	}
	b, err := Merge([]byte(original), site, []store.Article{post})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		TemplateName string `json:"templateName"`
		Articles     []struct {
			ID          string   `json:"id"`
			Attachments []string `json:"attachments"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.TemplateName != "Grid" || len(out.Articles) != 1 || out.Articles[0].ID != post.ID || len(out.Articles[0].Attachments) != 1 || out.Articles[0].Attachments[0] != "new.png" {
		t.Fatalf("incorrect metadata: %s", b)
	}
}

func TestReadImportsAttachmentMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sample.planet")
	os.Mkdir(dir, 0700)
	raw := strings.Replace(fixture, `"starType":0`, `"starType":0,"attachments":["photo.png"],"articleNumber":7`, 1)
	os.WriteFile(filepath.Join(dir, "planet.json"), []byte(raw), 0600)
	os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600)
	os.Mkdir(filepath.Join(dir, "22222222-2222-4222-8222-222222222222"), 0700)
	p, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Articles[0].Attachments) != 1 || p.Articles[0].Attachments[0] != "photo.png" || p.Articles[0].ArticleNumber == nil || *p.Articles[0].ArticleNumber != 7 {
		t.Fatalf("article: %+v", p.Articles[0])
	}
}

func TestImportedChangeWithSameAttachmentNamesRequiresRebuild(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", ContentRevision: 2}
	raw := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","about":"","templateName":"Plain","articles":[{"id":"22222222-2222-4222-8222-222222222222","title":"Post","content":"Body","attachments":["photo.png"]}]}`
	post := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Post", Content: "Body", Attachments: []string{"photo.png"}}
	if err := CheckUnchanged([]byte(raw), site, []store.Article{post}); err == nil {
		t.Fatal("edited imported site treated as unchanged")
	}
}

func TestMediaMetadataRoundTripsInBackup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "media.planet")
	os.MkdirAll(filepath.Join(dir, "22222222-2222-4222-8222-222222222222"), 0700)
	os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600)
	raw := strings.Replace(fixture, `"starType":0`, `"starType":0,"audioFilename":"song.m4a","videoFilename":"clip.mp4","attachments":["song.m4a","clip.mp4"]`, 1)
	os.WriteFile(filepath.Join(dir, "planet.json"), []byte(raw), 0600)
	parsed, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := parsed.Articles[0]
	if a.AudioFilename != "song.m4a" || a.VideoFilename != "clip.mp4" {
		t.Fatalf("metadata=%+v", a)
	}
	a.AudioFilename = ""
	a.Attachments = []string{"clip.mp4"}
	b, err := Merge(parsed.Raw, parsed.Site, []store.Article{a})
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Articles []map[string]json.RawMessage `json:"articles"`
	}
	json.Unmarshal(b, &value)
	if _, ok := value.Articles[0]["audioFilename"]; ok {
		t.Fatalf("deleted audio retained: %s", b)
	}
	if string(value.Articles[0]["videoFilename"]) != `"clip.mp4"` {
		t.Fatalf("video lost: %s", b)
	}
}

func TestPublicAudioMetadataRequiresRegisteredAudio(t *testing.T) {
	fields := map[string]json.RawMessage{"audioDuration": json.RawMessage(`42`), "hasAudio": json.RawMessage(`false`)}
	if err := CheckSimplePublicArticle(fields); err == nil {
		t.Fatal("orphan audio duration accepted")
	}
}
