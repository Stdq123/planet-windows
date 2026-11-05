package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"windows/internal/store"
)

func TestTurningOffNodePersistsAndPreventsAutomaticRestart(t *testing.T) {
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	if err := lib.SetIPFSEnabled(false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.ensureNode(true); err == nil || !strings.Contains(err.Error(), "已关闭") {
		t.Fatalf("automatic start was not blocked: %v", err)
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.IPFSDisabled() {
		t.Fatal("stopped preference was not saved")
	}
	status, err := lib.IPFSStatus()
	if err != nil || !status.Disabled || status.Running {
		t.Fatalf("unexpected status: %+v %v", status, err)
	}
}

func TestNodeControlStartsStopsAndReopensOwnedKubo(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("online Kubo required")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	lib := &Library{}
	t.Cleanup(lib.closeNode)
	if err := lib.SetIPFSEnabled(true); err != nil {
		t.Fatal(err)
	}
	status, err := lib.IPFSStatus()
	if err != nil || !status.Running || !status.Online || status.Gateway == "" {
		t.Fatalf("started: %+v %v", status, err)
	}
	if err := lib.SetIPFSEnabled(false); err != nil {
		t.Fatal(err)
	}
	status, err = lib.IPFSStatus()
	if err != nil || status.Running || !status.Disabled {
		t.Fatalf("stopped: %+v %v", status, err)
	}
	if err := lib.SetIPFSEnabled(true); err != nil {
		t.Fatal(err)
	}
	status, err = lib.IPFSStatus()
	if err != nil || !status.Running || status.Disabled {
		t.Fatalf("restarted: %+v %v", status, err)
	}
}
