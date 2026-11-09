//go:build windows && amd64

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryAfterRegistrationCommitAndAfterArchive(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-archive", true: "after-archive"}[archived], func(t *testing.T) {
			root, source, files := registeredFixture(t)
			state, _ := readRegistryStrings(stateKey)
			uninstall, _ := readRegistryStrings(uninstallKey)
			old, newFiles := map[string]string{}, map[string]string{}
			for n, v := range files {
				old[n] = state[n]
				newFiles[n] = v
			}
			old["Uninstall.exe"] = state["Uninstall.exe"]
			newFiles["Uninstall.exe"] = digest("new uninstaller")
			j := upgradeJournal{Format: 1, Version: releaseVersion, Old: old, New: newFiles, State: state, Uninstall: uninstall}
			b, _ := json.Marshal(j)
			if e := setRegistryStrings(updateKey, map[string]string{"Journal": string(b)}); e != nil {
				t.Fatal(e)
			}
			if e := checkInstall(root); e == nil {
				t.Fatal("pending anchor allowed fresh install")
			}
			if e := checkUninstall(root, files); e == nil {
				t.Fatal("pending anchor allowed uninstall")
			}
			interrupted := errors.New("process stopped after registration")
			e := applyPayload(root, source, j, func() error {
				next := map[string]string{}
				for n, v := range state {
					next[n] = v
				}
				for n, v := range newFiles {
					next[n] = v
				}
				if e := setRegistryStrings(stateKey, next); e != nil {
					return e
				}
				if e := setRegistryStrings(uninstallKey, map[string]string{"DisplayVersion": releaseVersion}); e != nil {
					return e
				}
				if !archived {
					return interrupted
				}
				return nil
			}, nil)
			if e != nil && !errors.Is(e, interrupted) {
				t.Fatal(e)
			}
			if e := checkIdenticalInstall(root, files); e == nil {
				t.Fatal("identical install skipped recovery")
			}
			if e := applyUpgrade(root, source, files); e != nil {
				t.Fatal(e)
			}
			if e := checkIdenticalInstall(root, files); e != nil {
				t.Fatal(e)
			}
			if e := checkUninstall(root, files); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func isolatedKeys(t *testing.T) {
	t.Helper()
	b := make([]byte, 12)
	rand.Read(b)
	base := `Software\PlanetWindows\UpgradeTests-` + hex.EncodeToString(b)
	oldState, oldUninstall, oldUpdate, oldVersion := stateKey, uninstallKey, updateKey, releaseVersion
	stateKey, uninstallKey, updateKey = base+`\State`, base+`\Uninstall`, base+`\Transaction`
	releaseVersion = "0.3.0"
	t.Cleanup(func() {
		for _, key := range []string{stateKey, uninstallKey, updateKey} {
			registry.DeleteKey(registry.CURRENT_USER, key)
		}
		registry.DeleteKey(registry.CURRENT_USER, base)
		stateKey, uninstallKey, updateKey, releaseVersion = oldState, oldUninstall, oldUpdate, oldVersion
	})
}
func registeredFixture(t *testing.T) (string, string, map[string]string) {
	t.Helper()
	isolatedKeys(t)
	root, source := t.TempDir(), t.TempDir()
	old, files := map[string]string{}, map[string]string{}
	for _, name := range []string{"Planet Windows.exe", "ipfs.exe", "icon.png", "kubo-LICENSE.txt", "kubo-LICENSE-MIT.txt", "kubo-LICENSE-APACHE.txt", "LICENSE-PLANET.txt", "THIRD-PARTY-NOTICES.txt", "KUBO-BUILDINFO.txt", "KUBO-DEPENDENCY-LICENSES.txt"} {
		if e := os.WriteFile(filepath.Join(root, name), []byte("old:"+name), 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(source, name), []byte("new:"+name), 0600); e != nil {
			t.Fatal(e)
		}
		old[name] = digest("old:" + name)
		files[name] = digest("new:" + name)
	}
	os.WriteFile(filepath.Join(root, "Uninstall.exe"), []byte("old uninstaller"), 0600)
	os.WriteFile(filepath.Join(source, "Uninstall.exe"), []byte("new uninstaller"), 0600)
	if e := record(root, old); e != nil {
		t.Fatal(e)
	}
	if e := setRegistryStrings(uninstallKey, map[string]string{"InstallLocation": root, "DisplayVersion": "0.2.0"}); e != nil {
		t.Fatal(e)
	}
	return root, source, files
}
func TestRegisteredCrossVersionUpgradeAndRepeat(t *testing.T) {
	root, source, files := registeredFixture(t)
	if e := applyUpgrade(root, source, files); e != nil {
		t.Fatal(e)
	}
	if e := checkIdenticalInstall(root, files); e != nil {
		t.Fatal(e)
	}
	k, e := readRegistryStrings(uninstallKey)
	if e != nil || k["DisplayVersion"] != "0.3.0" {
		t.Fatal(k, e)
	}
	if exists, e := existsKey(updateKey); e != nil || exists {
		t.Fatal("transaction not cleared", e)
	}
}

func TestRegisteredPrereleasePromotionAndDowngrade(t *testing.T) {
	root, source, files := registeredFixture(t)
	if err := setRegistryStrings(uninstallKey, map[string]string{"DisplayVersion": "0.0.1-alpha.1"}); err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{"0.0.1-alpha.2", "0.0.1-alpha.10", "0.0.1"} {
		releaseVersion = next
		if err := applyUpgrade(root, source, files); err != nil {
			t.Fatalf("upgrade to %s: %v", next, err)
		}
		entry, err := readRegistryStrings(uninstallKey)
		if err != nil || entry["DisplayVersion"] != next {
			t.Fatalf("version registration: %v, %v", entry, err)
		}
		if err := checkIdenticalInstall(root, files); err != nil {
			t.Fatalf("repeat %s: %v", next, err)
		}
	}
	releaseVersion = "0.0.1-alpha.10"
	if err := applyUpgrade(root, source, files); err == nil {
		t.Fatal("stable-to-alpha downgrade accepted")
	}
	entry, err := readRegistryStrings(uninstallKey)
	if err != nil || entry["DisplayVersion"] != "0.0.1" {
		t.Fatalf("failed downgrade changed registration: %v, %v", entry, err)
	}
	if exists, err := existsKey(updateKey); err != nil || exists {
		t.Fatalf("failed downgrade created a transaction: %v", err)
	}
}

func TestIdenticalPayloadAtAnotherReleaseStillRequiresVersionTransition(t *testing.T) {
	root, source, files := registeredFixture(t)
	if e := applyUpgrade(root, source, files); e != nil {
		t.Fatal(e)
	}
	releaseVersion = "0.4.0"
	if e := checkIdenticalInstall(root, files); e == nil {
		t.Fatal("same files skipped a required installer-version update")
	}
	if e := applyUpgrade(root, source, files); e != nil {
		t.Fatal(e)
	}
	entry, _ := readRegistryStrings(uninstallKey)
	if entry["DisplayVersion"] != "0.4.0" {
		t.Fatal("installer version was not advanced")
	}
}

func TestInterruptedEmptyAnchorCanRetryWithoutChangingOwnership(t *testing.T) {
	root, source, files := registeredFixture(t)
	k, _, e := registry.CreateKey(registry.CURRENT_USER, updateKey, registry.SET_VALUE)
	if e != nil {
		t.Fatal(e)
	}
	k.Close()
	if e = applyUpgrade(root, source, files); e != nil {
		t.Fatal(e)
	}
	if e = checkIdenticalInstall(root, files); e != nil {
		t.Fatal(e)
	}
}
func TestLegacyOrExpandedPayloadRefusedBeforeCreatingTransaction(t *testing.T) {
	for _, kind := range []string{"legacy", "expanded"} {
		t.Run(kind, func(t *testing.T) {
			root, source, files := registeredFixture(t)
			if kind == "legacy" {
				k, e := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.SET_VALUE)
				if e != nil {
					t.Fatal(e)
				}
				k.DeleteValue("TransactionProtocol")
				k.Close()
			} else {
				files["future.dll"] = digest("new")
				os.WriteFile(filepath.Join(source, "future.dll"), []byte("new"), 0600)
			}
			if e := applyUpgrade(root, source, files); e == nil {
				t.Fatal("unsupported update accepted")
			}
			if exists, e := existsKey(updateKey); e != nil || exists {
				t.Fatal("failed validation left transaction", e)
			}
			b, _ := os.ReadFile(filepath.Join(root, "Planet Windows.exe"))
			if string(b) != "old:Planet Windows.exe" {
				t.Fatal("old executable changed")
			}
		})
	}
}
func TestUnregisteredForgedJournalCannotAuthorizeUpdate(t *testing.T) {
	isolatedKeys(t)
	root, source := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(root, ".planet-update"), 0700)
	files := map[string]string{"Planet Windows.exe": digest("new")}
	os.WriteFile(filepath.Join(root, "Planet Windows.exe"), []byte("user file"), 0600)
	os.WriteFile(filepath.Join(source, "Planet Windows.exe"), []byte("new"), 0600)
	os.WriteFile(filepath.Join(source, "Uninstall.exe"), []byte("new uninstaller"), 0600)
	j := upgradeJournal{Format: 1, Version: releaseVersion, Old: map[string]string{"Planet Windows.exe": digest("user file")}, New: files, State: map[string]string{"InstallLocation": root}, Uninstall: map[string]string{"InstallLocation": root}}
	b, _ := json.Marshal(j)
	os.WriteFile(filepath.Join(root, ".planet-update", "journal.json"), b, 0600)
	if e := applyUpgrade(root, source, files); e == nil {
		t.Fatal("forged journal authorized update")
	}
	b, _ = os.ReadFile(filepath.Join(root, "Planet Windows.exe"))
	if string(b) != "user file" {
		t.Fatal("unregistered content changed")
	}
}
