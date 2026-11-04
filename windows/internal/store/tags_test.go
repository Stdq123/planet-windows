package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImportedArticleTagsSurviveLibraryReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	article := Article{ID: postID, SiteID: siteID, Title: "Post", Tags: map[string]string{"go": "Go language"}}
	if err = s.Import(Site{ID: siteID, IPNS: "name", Imported: true}, []Article{article}, json.RawMessage(`{"articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListArticles(siteID)
	if err != nil || len(posts) != 1 || posts[0].Tags["go"] != "Go language" {
		t.Fatalf("tags lost after reopen: %+v %v", posts, err)
	}
}

func TestArticleTagsSaveAtomicallyAndRejectUnsafeKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	method := reflect.ValueOf(s).MethodByName("SaveArticleWithTags")
	if !method.IsValid() {
		t.Fatal("article tag save operation is unavailable")
	}
	call := func(id, title, tags string) (Article, error) {
		out := method.Call([]reflect.Value{reflect.ValueOf(site.ID), reflect.ValueOf(id), reflect.ValueOf(title), reflect.ValueOf("body"), reflect.ValueOf(tags)})
		article := out[0].Interface().(Article)
		if out[1].IsNil() {
			return article, nil
		}
		return article, out[1].Interface().(error)
	}
	post, err := call("", "First", `{"go":"Go"}`)
	if err != nil {
		t.Fatal(err)
	}
	if post.Tags["go"] != "Go" {
		t.Fatalf("tags missing: %+v", post)
	}
	if _, err = call(post.ID, "MUST NOT SAVE", `{"../unsafe":"bad"}`); err == nil {
		t.Fatal("unsafe tag accepted")
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || posts[0].Title != "First" || posts[0].Tags["go"] != "Go" {
		t.Fatalf("invalid edit partially persisted: %+v %v", posts, err)
	}
	_, _, _, err = s.PublicationSnapshot(site.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftTagsRemainPrivateUntilAtomicPublication(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.SaveArticleWithTags(site.ID, "", "Post", "public", `{"go":"Go"}`)
	if err != nil {
		t.Fatal(err)
	}
	method := reflect.ValueOf(s).MethodByName("SaveDraftWithTags")
	if !method.IsValid() {
		t.Fatal("draft tag save operation is unavailable")
	}
	out := method.Call([]reflect.Value{reflect.ValueOf(site.ID), reflect.ValueOf(""), reflect.ValueOf(post.ID), reflect.ValueOf("Draft"), reflect.ValueOf("private"), reflect.ValueOf(`{"draft":"Private label"}`)})
	if !out[1].IsNil() {
		t.Fatal(out[1].Interface())
	}
	draft := out[0].Interface().(Draft)
	_, posts, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || len(posts) != 1 || posts[0].Tags["go"] != "Go" || posts[0].Tags["draft"] != "" {
		t.Fatalf("draft leaked: %+v %v", posts, err)
	}
	published, err := s.PublishDraft(site.ID, draft.ID, "Draft", "private")
	if err != nil {
		t.Fatal(err)
	}
	if published.Tags["draft"] != "Private label" || len(published.Tags) != 1 {
		t.Fatalf("draft tags not committed: %+v", published)
	}
}

func TestLegacyImportedLibraryRecoversTagsForEditing(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	article := Article{ID: postID, SiteID: siteID, Title: "Post"}
	raw := json.RawMessage(`{"articles":[{"id":"` + postID + `","tags":{"中文":"中文标签"}}]}`)
	if err = s.Import(Site{ID: siteID, IPNS: "name", Imported: true}, []Article{article}, raw); err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListArticles(siteID)
	if err != nil || len(posts) != 1 || posts[0].Tags["中文"] != "中文标签" {
		t.Fatalf("original tags not visible: %+v %v", posts, err)
	}
}

func TestMalformedImportedTagsDoNotBlockEntireLibrary(t *testing.T) {
	dir := t.TempDir()
	const siteID = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	data := `{"version":3,"sites":[{"id":"` + siteID + `","name":"Imported","ipns":"key","imported":true}],"articles":[{"id":"` + post + `","siteId":"` + siteID + `","title":"Post","created":"2020-01-01T00:00:00Z","modified":"2020-01-01T00:00:00Z"}],"backupMetadata":{"` + siteID + `":{"articles":[{"id":"` + post + `","tags":["old-unrecognized"]}]}}}`
	if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("one old unsupported site blocked library: %v", err)
	}
	_, _, raw, err := s.PublicationSnapshot(siteID)
	if err != nil || !strings.Contains(string(raw), `["old-unrecognized"]`) {
		t.Fatalf("original data lost: %s %v", raw, err)
	}
}

func TestUnsupportedImportedTagsDoNotBlockUnchangedDraftPublication(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"articles":[{"id":"` + postID + `","tags":["private-old"]}]}`)
	if err = s.Import(Site{ID: siteID, IPNS: "name", Imported: true}, []Article{{ID: postID, SiteID: siteID, Title: "Post"}}, raw); err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(siteID, "", postID, "Updated", "content")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PublishDraftPreparedWithTags(siteID, draft.ID, "Updated", "content", `{"new":"New"}`, nil); err == nil {
		t.Fatal("unsupported site accepted tag edits")
	}
	article, err := s.PublishDraft(siteID, draft.ID, "Updated", "content")
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "Updated" || len(article.Tags) != 0 {
		t.Fatalf("unexpected published article: %+v", article)
	}
	_, _, savedRaw, err := s.PublicationSnapshot(siteID)
	if err != nil || string(savedRaw) != string(raw) {
		t.Fatalf("raw tags changed: %s %v", savedRaw, err)
	}
}
