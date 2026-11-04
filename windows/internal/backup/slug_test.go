package backup

import (
	"encoding/json"
	"fmt"
	"testing"

	"windows/internal/store"
)

func TestImportedSlugValidation(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	const otherID = "33333333-3333-4333-8333-333333333333"
	site := store.Site{ID: siteID, Name: "Site"}
	for _, tc := range []struct {
		name, slug, link, other string
		valid                   bool
	}{
		{"valid", "hello-world", "/hello-world/", "", true},
		{"leading hyphen allowed by Mac", "-hello", "/-hello/", "", true},
		{"legacy UUID link", "hello-world", "/" + postID + "/", "", true},
		{"traversal", "../secret", "/../secret/", "", false},
		{"reserved assets", "assets", "/assets/", "", false},
		{"article collision", otherID, "/" + otherID + "/", "", false},
		{"unrelated link", "hello", "/other/", "", false},
		{"duplicate slug", "hello", "/hello/", `,{"id":"` + otherID + `","link":"/HELLO/","slug":"HELLO"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(fmt.Sprintf(`{"id":"%s","name":"Site","about":"","templateName":"Plain","articles":[{"id":"%s","link":%q,"slug":%q}%s]}`, siteID, postID, tc.link, tc.slug, tc.other))
			articles := []store.Article{{ID: postID, SiteID: siteID}}
			if tc.other != "" || tc.name == "article collision" {
				articles = append(articles, store.Article{ID: otherID, SiteID: siteID})
			}
			err := CheckSimplePlain(raw, site, articles)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
