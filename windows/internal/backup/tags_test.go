package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"windows/internal/store"
)

func TestImportedTagPathsRejectTraversalAndGeneratedFiles(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	for _, tc := range []struct {
		key   string
		valid bool
	}{
		{"go", true}, {"../secret", false}, {"index", false}, {"tags", false}, {"archive", false}, {"page2", false}, {"CON", false}, {"a/b", false},
		{"中文", true},
		{"COM¹", false}, {"LPT²", false}, {"com³", false},
		{"tagſ", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			raw := json.RawMessage(fmt.Sprintf(`{"id":"%s","name":"Site","about":"","templateName":"Plain","tags":{%q:"Label"},"articles":[{"id":"%s","link":"/%s/","tags":{%q:"Label"}}]}`, siteID, tc.key, post, post, tc.key))
			err := CheckSimplePlain(raw, store.Site{ID: siteID, Name: "Site"}, []store.Article{{ID: post, SiteID: siteID}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestMergeRemovesDeletedArticleTagsFromSiteIndex(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	const one = "22222222-2222-4222-8222-222222222222"
	const two = "33333333-3333-4333-8333-333333333333"
	raw := json.RawMessage(`{"tags":{"go":"Go","old":"Old"},"articles":[{"id":"` + one + `","tags":{"go":"Go"}},{"id":"` + two + `","tags":{"old":"Old"}}]}`)
	site := store.Site{ID: siteID, Name: "Site", Created: time.Unix(0, 0)}
	article := store.Article{ID: one, SiteID: siteID, Title: "Kept", Created: time.Unix(0, 0), Modified: time.Unix(0, 0)}
	b, err := Merge(raw, site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Tags     map[string]string `json:"tags"`
		Articles []struct {
			Tags map[string]string `json:"tags"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tags) != 1 || result.Tags["go"] != "Go" || len(result.Articles) != 1 || result.Articles[0].Tags["go"] != "Go" {
		t.Fatalf("stale tags: %s", b)
	}
}

func TestImportedTagPathsRejectUnicodeCaseFoldCollision(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"id":"` + siteID + `","name":"Site","templateName":"Plain","tags":{"σ":"Sigma","ς":"Final sigma"},"articles":[{"id":"` + post + `","link":"/` + post + `/","tags":{"σ":"Sigma","ς":"Final sigma"}}]}`)
	if err := CheckSimplePlain(raw, store.Site{ID: siteID, Name: "Site"}, []store.Article{{ID: post, SiteID: siteID}}); err == nil {
		t.Fatal("Windows Unicode case collision accepted")
	}
}

func TestImportedTagLabelsPreserveOriginalSiteValueUntilEdited(t *testing.T) {
	raw := json.RawMessage(`{"tags":{"go":"GO"},"articles":[{"id":"POST","tags":{"go":"Go"}}]}`)
	article := store.Article{ID: "POST", Tags: map[string]string{"go": "Go"}}
	labels, _, err := ImportedTags(raw, []store.Article{article})
	if err != nil || labels["go"] != "GO" {
		t.Fatalf("original site label changed: %v %v", labels, err)
	}
	article.Tags["go"] = "Go language"
	labels, _, err = ImportedTags(raw, []store.Article{article})
	if err != nil || labels["go"] != "Go language" {
		t.Fatalf("edited label not used: %v %v", labels, err)
	}
}

func TestMalformedTagsRemainInUnchangedImportedBackup(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	dir := filepath.Join(t.TempDir(), "source.planet")
	if err := os.MkdirAll(filepath.Join(dir, post), 0700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"key","created":1,"templateName":"Plain","articles":[{"id":"` + post + `","title":"Post","content":"Body","created":1,"tags":["unsupported"]}]}`
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckUnchanged(parsed.Raw, parsed.Site, parsed.Articles); err != nil {
		t.Fatalf("unchanged source blocked: %v", err)
	}
	if string(parsed.Raw) != raw || parsed.Articles[0].Tags != nil {
		t.Fatalf("unsupported data altered: %s %+v", parsed.Raw, parsed.Articles)
	}
}
