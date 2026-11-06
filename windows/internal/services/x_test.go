package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var testXCredentials = XCredentials{APIKey: "consumer-key", APISecret: "consumer-secret", AccessToken: "123-access-token", AccessTokenSecret: "access-token-secret"}

// OAuth Core 1.0a, Appendix A.5: a published, independently derived signature.
func TestXOAuthSignatureMatchesOfficialVector(t *testing.T) {
	creds := XCredentials{"dpf43f3p2l4k3l03", "kd94hf93k423kf44", "nnch734d00sl2jdk", "pfkkdhi9sl3r4s00"}
	header, err := xOAuthHeader("GET", "http://photos.example.net/photos?file=vacation.jpg&size=original", creds, "kllo9940pd9333jh", "1191242096")
	if err != nil || !strings.Contains(header, `oauth_signature="tR3%2BTy81lMeYAr%2FFid0kMTYa%2FWM%3D"`) {
		t.Fatalf("incorrect signature: %s, %v", header, err)
	}
	if strings.Contains(header, creds.APISecret) || strings.Contains(header, creds.AccessTokenSecret) || strings.Contains(header, "file=") {
		t.Fatal("signature header exposed secrets or query parameters")
	}
}

func TestXCreatesJSONPostWithOAuthAndReadsNumericID(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/2/tweets" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected post request")
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "OAuth ") || !strings.Contains(header, `oauth_consumer_key="consumer-key"`) || !strings.Contains(header, `oauth_token="123-access-token"`) || !strings.Contains(header, `oauth_signature="`) {
			t.Error("missing OAuth authorization")
		}
		if strings.Contains(header, testXCredentials.APISecret) || strings.Contains(header, testXCredentials.AccessTokenSecret) {
			t.Error("sent plaintext signing secret")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["text"] != "标题\nhttps://example.com/article/" {
			t.Errorf("incorrect payload: %#v", body)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"data":{"id":"18446744073709551615","text":"标题"}}`)
	}))
	defer server.Close()
	id, err := newClient(server.Client()).createXPost(server.URL+"/2/tweets", testXCredentials, "标题\nhttps://example.com/article/")
	if err != nil || id != "18446744073709551615" || calls != 1 {
		t.Fatalf("post: %q %v, calls %d", id, err, calls)
	}
}

func TestXFailuresDistinguishRejectionFromUncertainOutcome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		rejected bool
		message  string
	}{
		{"unauthorized", 401, `{"detail":"consumer-secret"}`, true, "凭据"},
		{"forbidden", 403, `{"detail":"access-token-secret"}`, true, "权限"},
		{"billing", 402, `{"title":"CreditsDepleted"}`, true, "额度"},
		{"rate limit", 429, `{"detail":"consumer-secret"}`, true, "频繁"},
		{"bad request", 400, `{"detail":"consumer-secret"}`, true, "拒绝"},
		{"server error", 503, `{"detail":"consumer-secret"}`, false, "不确定"},
		{"malformed success", 201, `{"data":`, false, "不确定"},
		{"missing id", 201, `{"data":{"text":"ok"}}`, false, "不确定"},
		{"unsafe id", 201, `{"data":{"id":"../secret"}}`, false, "不确定"},
		{"overflow id", 201, `{"data":{"id":"18446744073709551616"}}`, false, "不确定"},
		{"oversized success", 201, strings.Repeat("x", (256<<10)+1), false, "不确定"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := newClient(server.Client()).createXPost(server.URL, testXCredentials, "Hello")
			if err == nil || XPostRejected(fmt.Errorf("wrapped: %w", err)) != tc.rejected || !strings.Contains(err.Error(), tc.message) || calls != 1 {
				t.Fatalf("incorrect outcome: %v, calls %d", err, calls)
			}
			if strings.Contains(err.Error(), "consumer-secret") || strings.Contains(err.Error(), "access-token-secret") {
				t.Fatal("raw response leaked into error")
			}
		})
	}
	if XPostRejected(errors.New("arbitrary failure")) || XPostRejected(nil) {
		t.Fatal("arbitrary error classified as definite rejection")
	}
}

func TestXNeverFollowsRedirectsOrRetriesLostResponse(t *testing.T) {
	stolen := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { stolen++ }))
	defer target.Close()
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer server.Close()
	c := newClient(server.Client())
	for _, path := range []string{"/redirect", "/lost"} {
		if _, err := c.createXPost(server.URL+path, testXCredentials, "Hello"); err == nil || XPostRejected(err) {
			t.Fatalf("expected uncertain result: %v", err)
		}
	}
	if stolen != 0 || requests != 2 {
		t.Fatalf("redirect/retry: %d, %d", stolen, requests)
	}
}

func TestXTextKeepsPermalinkAndFitsWeightedLimit(t *testing.T) {
	link := "https://example.com/articles/" + strings.Repeat("long-", 60)
	for _, tc := range []struct{ name, title, wantTitle string }{
		{"whitespace", "  Hello\n 世界\t ", "Hello 世界"},
		{"empty", " \n\t", ""},
		{"latin", strings.Repeat("a", 300), strings.Repeat("a", 254) + "…"},
		{"cjk", strings.Repeat("文", 200), strings.Repeat("文", 127) + "…"},
		{"emoji", strings.Repeat("🚀", 200), strings.Repeat("🚀", 127) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := XPostText(tc.title, link)
			want := tc.wantTitle + "\n" + link
			if tc.wantTitle == "" {
				want = link
			}
			if err != nil || got != want || !utf8.ValidString(got) {
				t.Fatalf("text: %q %v", got, err)
			}
		})
	}
	for _, link := range []string{"http://example.com/a", "https://user:secret@example.com/a", "https:///a", "https://example.com/a\nsecret", "https://example.com/a b", "https://localhost/a"} {
		if _, err := XPostText("title", link); err == nil {
			t.Errorf("accepted invalid permalink %q", link)
		}
	}
	if _, err := XPostText(string([]byte{0xff}), "https://example.com/a"); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
	got, err := XPostText(strings.Repeat("a.co ", 50), "https://example.com/a")
	if err != nil || len(strings.Fields(strings.Split(got, "\n")[0])) > 11 {
		t.Fatalf("short title URLs can exceed URL expansion budget: %q %v", got, err)
	}
}

func TestXCredentialsStoredPrivatelyAndValidatedOnLoad(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Name: "X", Token: testXCredentials.AccessToken, XCredentials: &testXCredentials, AutoSync: true, XEnabledAt: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)}
	if err := SaveConfig(root, "site-1", "x", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(root, "site-1", "x")
	if err != nil || got.XCredentials == nil || *got.XCredentials != testXCredentials || !got.AutoSync || !got.XEnabledAt.Equal(cfg.XEnabledAt) {
		t.Fatalf("roundtrip: %#v %v", got, err)
	}
	path := filepath.Join(root, "integrations", "x", "site-1.dat")
	sealed, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(sealed), "consumer-secret") || strings.Contains(string(sealed), "access-token-secret") {
		t.Fatalf("plaintext secrets: %v", err)
	}
	missingToken := cfg
	missingToken.Token = ""
	if err = SaveConfig(root, "site-1", "x", missingToken); err == nil {
		t.Fatal("credential-bearing config with missing token silently deleted existing config")
	}
	if got, err = LoadConfig(root, "site-1", "x"); err != nil || got.XCredentials == nil {
		t.Fatalf("invalid save removed credentials: %#v %v", got, err)
	}
	for _, bad := range []XCredentials{{}, {APIKey: "key", APISecret: "secret", AccessToken: "token"}, {APIKey: "key\n", APISecret: "secret", AccessToken: "token", AccessTokenSecret: "secret"}, {APIKey: "key", APISecret: "secret", AccessToken: "to ken", AccessTokenSecret: "secret"}} {
		if ValidXCredentials(bad) {
			t.Error("accepted malformed credentials")
		}
		badCfg := cfg
		badCfg.XCredentials = &bad
		if SaveConfig(root, "site-1", "x", badCfg) == nil {
			t.Fatal("saved malformed credentials")
		}
	}
	badCfg := cfg
	badCfg.XCredentials = nil
	raw, _ := json.Marshal(badCfg)
	sealed, err = protect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, sealed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(root, "site-1", "x"); err == nil {
		t.Fatal("loaded incomplete credentials")
	}
}

func TestXPostRecordsArePrivateDurableAndRemovedOnlyForTheirSite(t *testing.T) {
	root := t.TempDir()
	if got, err := LoadXPost(root, "site-1", "article-1"); err != nil || got != (XPostRecord{}) {
		t.Fatalf("missing: %#v %v", got, err)
	}
	for _, status := range []string{"pending", "failed", "sent"} {
		record := XPostRecord{Status: status}
		if status == "sent" {
			record.PostID = "12345"
			record.URL = "https://x.com/i/web/status/12345"
		}
		if err := SaveXPost(root, "site-1", "article-1", record); err != nil {
			t.Fatal(err)
		}
		got, err := LoadXPost(root, "site-1", "article-1")
		if err != nil || got != record {
			t.Fatalf("record: %#v %v", got, err)
		}
	}
	if err := SaveXPost(root, "site-2", "article-1", XPostRecord{Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "integrations", "x-post"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("records: %v %v", entries, err)
	}
	for _, entry := range entries {
		if len(entry.Name()) != 68 || strings.Contains(entry.Name(), "site") {
			t.Fatal("record key is not hashed")
		}
		data, err := os.ReadFile(filepath.Join(root, "integrations", "x-post", entry.Name()))
		if err != nil || strings.Contains(string(data), "article-1") || strings.Contains(string(data), "x.com") {
			t.Fatalf("plaintext record: %v", err)
		}
	}
	if err := DeleteXPostsForSite(root, "site-1"); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadXPost(root, "site-1", "article-1"); err != nil || got.Status != "" {
		t.Fatalf("not removed: %#v %v", got, err)
	}
	if got, err := LoadXPost(root, "site-2", "article-1"); err != nil || got.Status != "pending" {
		t.Fatalf("other site removed: %#v %v", got, err)
	}
}

func TestXPostRecordsRejectUnsafeValues(t *testing.T) {
	root := t.TempDir()
	for _, record := range []XPostRecord{
		{Status: "unknown"}, {Status: "pending", PostID: "123"}, {Status: "sent"},
		{Status: "sent", PostID: "../bad", URL: "https://x.com/i/web/status/123"},
		{Status: "sent", PostID: "123", URL: "https://evil.example/status/123"},
		{Status: "sent", PostID: "123", URL: "https://x.com/i/web/status/124"},
		{Status: "failed", Error: "raw response with private-token"},
	} {
		if err := SaveXPost(root, "site-1", "article-1", record); err == nil {
			t.Errorf("accepted %#v", record)
		}
	}
	if err := SaveXPost(root, "../site", "article-1", XPostRecord{Status: "pending"}); err == nil {
		t.Fatal("accepted unsafe site")
	}
	if err := DeleteXPostsForSite(root, "../site"); err == nil {
		t.Fatal("accepted unsafe delete")
	}
	if _, err := LoadXPost(root, "site-1", "../article"); err == nil {
		t.Fatal("accepted unsafe article")
	}
}

func TestXOAuthEscapesHeaderAndQueryValues(t *testing.T) {
	creds := testXCredentials
	creds.APIKey = "key+with/slash"
	header, err := xOAuthHeader("POST", "https://api.x.com/2/tweets", creds, "nonce", "123")
	if err != nil || !strings.Contains(header, `oauth_consumer_key="key%2Bwith%2Fslash"`) {
		t.Fatalf("header: %s %v", header, err)
	}
	for _, part := range strings.Split(strings.TrimPrefix(header, "OAuth "), ", ") {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			t.Fatal("invalid OAuth header")
		}
		if _, err := url.QueryUnescape(strings.Trim(pair[1], `"`)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestXTextNormalizesBeforeCountingAndRejectsUnshortenableLinks(t *testing.T) {
	link := "https://example.com/a"
	got, err := XPostText("Cafe\u0301", link)
	if err != nil || got != "Café\n"+link {
		t.Fatalf("NFC: %q %v", got, err)
	}
	// U+0344 has a two-code-point NFC decomposition. Counting first would let
	// X normalize 100 spaced marks into 299 characters and reject the post.
	got, err = XPostText(strings.Repeat("\u0344 ", 100), link)
	if err != nil || got != strings.Repeat("\u0308\u0301 ", 84)+"\u0308\u0301…\n"+link {
		t.Fatalf("NFC expansion: %q %v", got, err)
	}
	if _, err = XPostText("title", "https://example.com/"+strings.Repeat("a", 4096)); err == nil {
		t.Fatal("accepted URL longer than X extraction limit")
	}
}

func TestXErrorMessagesCannotPersistRawProviderData(t *testing.T) {
	if got := XPostMessage(errors.New("consumer-secret response body")); got != "X 请求结果不确定，请先到 X 检查后再重试。" {
		t.Fatalf("unsafe message %q", got)
	}
	root := t.TempDir()
	record := XPostRecord{Status: "failed", Error: XPostMessage(xResponseError(http.StatusUnauthorized))}
	if err := SaveXPost(root, "site-1", "article-1", record); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadXPost(root, "site-1", "article-1"); err != nil || got != record {
		t.Fatalf("error record: %#v %v", got, err)
	}
}

func TestXPostRecordsRejectSwappedIdentityAndUnexpectedCredentials(t *testing.T) {
	root := t.TempDir()
	if err := SaveXPost(root, "site-1", "article-1", XPostRecord{Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	id, err := xPostRecordID("site-1", "article-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{
		{Name: "site-2", Server: "article-1", Token: "pending"},
		{Name: "site-1", Server: "article-1", Token: "pending", XCredentials: &testXCredentials},
	} {
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := protect(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "integrations", "x-post", id+".dat"), sealed, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadXPost(root, "site-1", "article-1"); err == nil {
			t.Fatal("accepted corrupt record")
		}
		if err = DeleteXPostsForSite(root, "site-1"); err == nil {
			t.Fatal("deleted unverifiable record")
		}
	}
}
