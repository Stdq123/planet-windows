package pinnable

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEndpointRequiresPinnableHTTPSAndNoEmbeddedCredentials(t *testing.T) {
	for _, raw := range []string{"http://pinnable.xyz/pin/abc", "https://evil.test/pin/abc", "https://pinnable.xyz.evil.test/pin/abc", "https://user:pass@pinnable.xyz/pin/abc", "https://pinnable.xyz/pin/abc?token=secret", "https://pinnable.xyz/other/abc"} {
		if _, err := ParseEndpoint(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if _, err := ParseEndpoint("https://dev.pinnable.xyz/pin/1234-abcd"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestAndStatusRejectRedirectsAndReportResults(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pin/test":
			w.WriteHeader(http.StatusAccepted)
		case "/pin/test/status":
			json.NewEncoder(w).Encode(map[string]any{"status": "pinned", "last_known_cid": "bafytest"})
		case "/redirect":
			http.Redirect(w, r, "/pin/test", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := newClient(server.Client())
	if err := c.request(server.URL + "/pin/test"); err != nil {
		t.Fatal(err)
	}
	status, err := c.status(server.URL + "/pin/test")
	if err != nil || status.Status != "pinned" || status.CID != "bafytest" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if err := c.request(server.URL + "/redirect"); err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestPinChecksReportedIdentityBeforeRequest(t *testing.T) {
	var reports string
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status") {
			json.NewEncoder(w).Encode(map[string]any{"status": "ready", "last_known_ipns": reports})
			return
		}
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	c := newClient(server.Client())
	endpoint := server.URL + "/pin/test"
	reports = "other"
	if err := c.pinForSite(endpoint, "k51site", true); err == nil {
		t.Fatal("mismatched site accepted")
	}
	if calls != 0 {
		t.Fatal("mismatched site was requested")
	}
	reports = ""
	if err := c.pinForSite(endpoint, "k51site", true); err == nil {
		t.Fatal("unknown site auto-pinned")
	}
	if calls != 0 {
		t.Fatal("unknown site was auto-requested")
	}
	reports = "/ipns/k51site"
	if err := c.pinForSite(endpoint, "k51site", true); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("matching site was not requested")
	}
}

func TestEndpointIsProtectedAtRestAndCanBeRemoved(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows DPAPI required")
	}
	root := t.TempDir()
	const site = "site-123"
	const endpoint = "https://dev.pinnable.xyz/pin/1234-abcd"
	if err := Save(root, site, endpoint); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "integrations", "pinnable", site+".dat"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), endpoint) {
		t.Fatal("endpoint stored in plaintext")
	}
	got, err := Load(root, site)
	if err != nil || got != endpoint {
		t.Fatalf("endpoint mismatch: %v", err)
	}
	if err := Save(root, site, ""); err != nil {
		t.Fatal(err)
	}
	got, err = Load(root, site)
	if err != nil || got != "" {
		t.Fatalf("endpoint retained: %v", err)
	}
	if err := Save(root, "../bad", endpoint); err == nil {
		t.Fatal("unsafe site ID accepted")
	}
}

func TestPushRecordSendsMultipartAndValidatesSizeAndStatus(t *testing.T) {
	var gotRecord []byte
	var gotContentType string
	statusCode := http.StatusAccepted
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pin/test/record" {
			http.NotFound(w, r)
			return
		}
		gotContentType = r.Header.Get("Content-Type")
		file, _, err := r.FormFile("record")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		gotRecord, _ = io.ReadAll(file)
		w.WriteHeader(statusCode)
	}))
	defer server.Close()

	c := newClient(server.Client())
	endpoint := server.URL + "/pin/test"
	record := []byte("signed-ipns-record")

	if err := c.pushRecord(endpoint, record); err != nil {
		t.Fatalf("pushRecord 202 failed: %v", err)
	}
	if !bytes.Equal(gotRecord, record) || !strings.HasPrefix(gotContentType, "multipart/form-data;") {
		t.Fatalf("unexpected multipart payload: record=%q contentType=%q", gotRecord, gotContentType)
	}

	statusCode = http.StatusOK
	if err := c.pushRecord(endpoint, record); err != nil {
		t.Fatalf("pushRecord 200 failed: %v", err)
	}

	statusCode = http.StatusBadRequest
	if err := c.pushRecord(endpoint, record); err == nil {
		t.Fatal("pushRecord 400 was accepted")
	}

	if err := c.pushRecord(endpoint, nil); err == nil {
		t.Fatal("empty record was accepted")
	}
	if err := c.pushRecord(endpoint, make([]byte, MaxRecordBytes+1)); err == nil {
		t.Fatal("oversized record was accepted")
	}
	if err := c.PushRecord("http://pinnable.xyz/pin/test", record); err == nil {
		t.Fatal("non-HTTPS endpoint was accepted")
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("secret endpoint token")
}
func TestNetworkErrorsDoNotExposeEndpoint(t *testing.T) {
	c := newClient(&http.Client{Transport: failingTransport{}})
	for _, err := range []error{
		c.request("https://pinnable.xyz/pin/secret-token"),
		func() error { _, err := c.status("https://pinnable.xyz/pin/secret-token"); return err }(),
		c.pushRecord("https://pinnable.xyz/pin/secret-token", []byte("record")),
	} {
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("credential leaked: %v", err)
		}
	}
}
