package services

import (
	"errors"
	"testing"
)

type fakeDeploymentRunner struct {
	calls int
	fail  bool
}

func (f *fakeDeploymentRunner) Cloudflare(dir string, cfg Config) (string, error) {
	f.calls++
	if f.fail {
		return "", errors.New("connection lost")
	}
	return "https://my-site.pages.dev/", nil
}
func (f *fakeDeploymentRunner) SSH(dir string, cfg Config) error { f.calls++; return nil }

func TestUncertainDeploymentBlocksAutomaticRetryUntilExplicitRetry(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Name: "my-site", Server: "0123456789abcdef0123456789abcdef", Token: "private", AutoDeploy: true}
	if err := SaveConfig(root, "site-1", "cloudflare", cfg); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDeploymentRunner{fail: true}
	if _, err := DeployConfigured(root, "site-1", "cloudflare", t.TempDir(), false, runner); err == nil {
		t.Fatal("expected uncertain failure")
	}
	runner.fail = false
	if _, err := DeployConfigured(root, "site-1", "cloudflare", t.TempDir(), false, runner); err == nil || runner.calls != 1 {
		t.Fatal("automatic retry was not blocked")
	}
	url, err := DeployConfigured(root, "site-1", "cloudflare", t.TempDir(), true, runner)
	if err != nil || url != "https://my-site.pages.dev/" || runner.calls != 2 {
		t.Fatalf("explicit retry failed: %s %v", url, err)
	}
	stored, err := LoadConfig(root, "site-1", "cloudflare")
	if err != nil || stored.PendingCID != "" || stored.LastURL != url {
		t.Fatalf("status not recorded: %+v %v", stored, err)
	}
}
