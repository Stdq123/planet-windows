package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeKeepsOriginalLibrarySnapshotAndBlocksOldReaders(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		dir := t.TempDir()
		original, _ := json.Marshal(map[string]any{"version": version, "sites": []any{}, "articles": []any{}})
		path := filepath.Join(dir, "library.json")
		if e := os.WriteFile(path, original, 0600); e != nil {
			t.Fatal(e)
		}
		s, e := Open(dir)
		if e != nil {
			t.Fatal(e)
		}
		if s.data.Version != 4 {
			t.Fatalf("old reader can still write version %d", s.data.Version)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "library-before-v4-*.json"))
		if len(files) != 1 {
			t.Fatalf("snapshot missing: %v", files)
		}
		b, e := os.ReadFile(files[0])
		if e != nil || !bytes.Equal(b, original) {
			t.Fatal("original snapshot changed")
		}
		if _, e = Open(dir); e != nil {
			t.Fatal(e)
		}
		files, _ = filepath.Glob(filepath.Join(dir, "library-before-v4-*.json"))
		if len(files) != 1 {
			t.Fatal("reopening created duplicate snapshot")
		}
	}
}

func TestFutureLibraryVersionRefusedWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	b := []byte(`{"version":5,"sites":[],"articles":[]}`)
	path := filepath.Join(dir, "library.json")
	os.WriteFile(path, b, 0600)
	if _, e := Open(dir); e == nil {
		t.Fatal("future library accepted")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, b) {
		t.Fatal("future library changed")
	}
}

func TestSnapshotNeverAcceptsSymbolicLinkToMutableLibrary(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library.json")
	snapshot := filepath.Join(root, "snapshot.json")
	b := []byte(`{"version":3,"sites":[],"articles":[]}`)
	if e := os.WriteFile(library, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(library, snapshot); e != nil {
		t.Skipf("symbolic links unavailable: %v", e)
	}
	if e := snapshotLibrary(snapshot, b); e == nil {
		t.Fatal("mutable symlink accepted as original snapshot")
	}
}
