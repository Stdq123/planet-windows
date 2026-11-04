package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilebasePinPostsCIDAndDoesNotFollowRedirect(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/stolen", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path != "/pins" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["cid"] != "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m" {
			t.Error("bad pin body")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		w.Write([]byte(`{"requestid":"request-1","status":"queued","pin":{"cid":"bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"}}`))
	}))
	defer server.Close()
	c := newClient(server.Client())
	id, err := c.pin(server.URL+"/pins", "secret", "site", "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m")
	if err != nil || id != "request-1" {
		t.Fatalf("pin: %s %v", id, err)
	}
	if _, err = c.pin(server.URL+"/redirect", "secret", "site", "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"); err == nil {
		t.Fatal("accepted redirect")
	}
	if calls != 2 {
		t.Fatal("followed redirect")
	}
}

func TestProtectedConfigurationRoundTripAndRemoval(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Token: "private-token", Name: "example.com"}
	if err := SaveConfig(root, "site-1", "plausible", cfg); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(root, "integrations", "plausible", "site-1.dat"))
	if err != nil || strings.Contains(string(bytes), cfg.Token) {
		t.Fatalf("plaintext credential or unreadable: %v", err)
	}
	got, err := LoadConfig(root, "site-1", "plausible")
	if err != nil || got != cfg {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err = SaveConfig(root, "site-1", "plausible", Config{}); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig(root, "site-1", "plausible")
	if err != nil || got.Token != "" {
		t.Fatalf("removal: %+v %v", got, err)
	}
	if err = SaveConfig(root, "../site", "filebase", cfg); err == nil {
		t.Fatal("accepted unsafe site")
	}
}

func TestPlausibleServerStoredWithProtectedCredentials(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Token: "private-token", Name: "example.com", Server: "https://stats.example.com:8443"}
	if err := SaveConfig(root, "site-1", "plausible", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(root, "site-1", "plausible")
	if err != nil || got.Server != cfg.Server {
		t.Fatalf("server lost: %+v %v", got, err)
	}
	cfg.Server = "http://evil.example"
	if err = SaveConfig(root, "site-1", "plausible", cfg); err == nil {
		t.Fatal("insecure server accepted")
	}
	got, err = LoadConfig(root, "site-1", "plausible")
	if err != nil || got.Server != "https://stats.example.com:8443" {
		t.Fatalf("existing config changed: %+v %v", got, err)
	}
}

type fakePinner struct{ calls int }
type failingPinner struct{ calls int }

func (f *failingPinner) PinFilebase(token, name, cid string) (string, error) {
	f.calls++
	return "", fmt.Errorf("response lost")
}

func (f *fakePinner) PinFilebase(token, name, cid string) (string, error) {
	f.calls++
	return "req-1", nil
}
func TestFilebasePinsNewVersionAndSkipsPreviouslySubmittedCID(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Token: "secret", Name: "my-site"}
	if err := SaveConfig(root, "site-1", "filebase", cfg); err != nil {
		t.Fatal(err)
	}
	f := &fakePinner{}
	cid := "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"
	if err := PinPublished(root, "site-1", cid, f); err != nil {
		t.Fatal(err)
	}
	if err := PinPublished(root, "site-1", cid, f); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("submitted %d times", f.calls)
	}
	cfg, err := LoadConfig(root, "site-1", "filebase")
	if err != nil || cfg.LastCID != cid || cfg.RequestID != "req-1" {
		t.Fatalf("lost state: %+v %v", cfg, err)
	}
}

func TestExplicitFilebaseRetryResubmitsSameVersion(t *testing.T) {
	root := t.TempDir()
	cid := "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"
	if err := SaveConfig(root, "site-1", "filebase", Config{Token: "secret", Name: "site", LastCID: cid, RequestID: "failed"}); err != nil {
		t.Fatal(err)
	}
	f := &fakePinner{}
	if err := RetryPublished(root, "site-1", cid, f); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("explicit retry did not submit")
	}
}

func TestUncertainFilebaseResponseBlocksAutomaticDuplicate(t *testing.T) {
	root := t.TempDir()
	cid := "bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"
	if err := SaveConfig(root, "site-1", "filebase", Config{Token: "secret", Name: "site"}); err != nil {
		t.Fatal(err)
	}
	f := &failingPinner{}
	if err := PinPublished(root, "site-1", cid, f); err == nil {
		t.Fatal("expected uncertain result")
	}
	if err := PinPublished(root, "site-1", cid, f); err == nil {
		t.Fatal("expected pending warning")
	}
	if f.calls != 1 {
		t.Fatalf("sent %d requests", f.calls)
	}
}

func TestFilebaseStatusReadsActualRemoteStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/pins/req-1" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("unexpected request")
		}
		w.Write([]byte(`{"requestid":"req-1","status":"pinned","pin":{"cid":"bafybeigdyrzt5sfp7udm7hu76uhy26nf3bokh67ouqlfbxywps53kuvy3m"}}`))
	}))
	defer server.Close()
	got, err := newClient(server.Client()).filebaseStatus(server.URL+"/pins", "secret", "req-1")
	if err != nil || got.Status != "pinned" || got.RequestID != "req-1" {
		t.Fatalf("status: %+v %v", got, err)
	}
	if _, err = newClient(server.Client()).filebaseStatus(server.URL+"/pins", "secret", "../bad"); err == nil {
		t.Fatal("accepted unsafe id")
	}
}

func TestPlausibleStatsUsesV2AndReturnsMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer private" {
			t.Error("invalid request")
		}
		var body struct {
			SiteID    string   `json:"site_id"`
			DateRange string   `json:"date_range"`
			Metrics   []string `json:"metrics"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.SiteID != "example.com" || body.DateRange != "7d" || len(body.Metrics) != 2 {
			t.Errorf("bad query: %+v", body)
		}
		w.Write([]byte(`{"results":[{"metrics":[12,34]}]}`))
	}))
	defer server.Close()
	stats, err := newClient(server.Client()).stats(server.URL, "private", "example.com")
	if err != nil || stats.Visitors != 12 || stats.Pageviews != 34 {
		t.Fatalf("bad stats: %+v %v", stats, err)
	}
}

func TestPlausibleStatsSupportsSelectedTimeRange(t *testing.T) {
	var ranges []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DateRange string `json:"date_range"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid JSON")
		}
		ranges = append(ranges, body.DateRange)
		_, _ = w.Write([]byte(`{"results":[{"metrics":[1,2]}]}`))
	}))
	defer server.Close()
	c := newClient(server.Client())
	for _, period := range []string{"day", "7d", "30d"} {
		if _, err := c.PlausibleStatsAtRange(server.URL, "private", "example.com", period); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.PlausibleStatsAtRange(server.URL, "private", "example.com", "invalid"); err == nil {
		t.Fatal("accepted invalid date range")
	}
	if strings.Join(ranges, ",") != "day,7d,30d" {
		t.Fatalf("unexpected ranges: %v", ranges)
	}
}

func TestSelfHostedPlausibleFallsBackToLegacyStatsOnlyWhenV2Unsupported(t *testing.T) {
	calls := []string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer private" {
			t.Error("authorization missing")
		}
		if r.URL.Path == "/api/v2/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/api/v1/stats/aggregate" || r.URL.Query().Get("site_id") != "example.com" || r.URL.Query().Get("period") != "day" || r.URL.Query().Get("metrics") != "visitors,pageviews" {
			t.Errorf("invalid fallback URL: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"results":{"visitors":{"value":4},"pageviews":{"value":9}}}`))
	}))
	defer server.Close()
	stats, err := newClient(server.Client()).PlausibleStatsAtRange(server.URL, "private", "example.com", "day")
	if err != nil || stats.Visitors != 4 || stats.Pageviews != 9 {
		t.Fatalf("legacy stats: %+v %v", stats, err)
	}
	if strings.Join(calls, ",") != "POST /api/v2/query,GET /api/v1/stats/aggregate" {
		t.Fatalf("unexpected requests: %v", calls)
	}
}

func TestPlausibleDoesNotFallbackOnAuthenticationFailure(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	if _, err := newClient(server.Client()).PlausibleStatsAtRange(server.URL, "bad", "example.com", "day"); err == nil {
		t.Fatal("accepted unauthorized request")
	}
	if calls != 1 {
		t.Fatalf("retried authentication failure %d times", calls)
	}
}

func TestPlausibleCustomServerValidatesOriginAndUsesTLS(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v2/query" || r.Header.Get("Authorization") != "Bearer private" {
			t.Errorf("unexpected target %s", r.URL.Path)
		}
		w.Write([]byte(`{"results":[{"metrics":[3,5]}]}`))
	}))
	defer server.Close()
	c := newClient(server.Client())
	stats, err := c.PlausibleStatsAt(server.URL, "private", "example.com")
	if err != nil || stats.Visitors != 3 || stats.Pageviews != 5 {
		t.Fatalf("stats %+v %v", stats, err)
	}
	for _, bad := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/path", "https://example.com?token=x", "https://example.com#x", "https://example.com.evil/../", "https://evil.test@localhost", "https://localhost:99999", "https://example.com:"} {
		if _, err = c.PlausibleStatsAt(bad, "private", "example.com"); err == nil {
			t.Errorf("accepted unsafe server %q", bad)
		}
	}
	if calls != 1 {
		t.Fatalf("unsafe URLs reached server: %d", calls)
	}
}

func TestPlausibleCustomServerDoesNotForwardKeyThroughRedirect(t *testing.T) {
	leaked := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	if _, err := newClient(source.Client()).PlausibleStatsAt(source.URL, "secret", "example.com"); err == nil {
		t.Fatal("accepted redirect")
	}
	if leaked {
		t.Fatal("token forwarded to redirect target")
	}
}
