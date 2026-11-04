//go:build windows && amd64

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var releaseVersion string // set by the build from release.json

var updateKey = `Software\PlanetWindows\UpdateTransaction`

// The journal names files, never arbitrary paths. Backups remain private local
// recovery evidence; they are not an application/data rollback mechanism.
type upgradeJournal struct {
	Format           int
	Version          string
	Old, New         map[string]string
	State, Uninstall map[string]string
}

func openOwned(path, want string) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.DELETE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(h), path)
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e == nil && (info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1) {
		e = errors.New("owned file is redirected or hard-linked")
	}
	if e == nil {
		hash := sha256.New()
		_, e = io.Copy(hash, f)
		if e == nil && !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), want) {
			e = fmt.Errorf("file changed: %s", path)
		}
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}

func moveOwned(f *os.File, to string) error {
	name := utf16.Encode([]rune(to))
	buf := make([]byte, 22+2*len(name))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(2*len(name)))
	for i, c := range name {
		binary.LittleEndian.PutUint16(buf[20+2*i:], c)
	}
	// ReplaceIfExists=false: collisions always preserve the destination.
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileRenameInfo, &buf[0], uint32(len(buf)))
}

func pinDirectories(path string) (func(), error) {
	var handles []windows.Handle
	close := func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}
	for current := path; ; current = filepath.Dir(current) {
		p, e := windows.UTF16PtrFromString(current)
		if e != nil {
			close()
			return nil, e
		}
		h, e := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			close()
			return nil, e
		}
		handles = append(handles, h)
		var info windows.ByHandleFileInformation
		if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			close()
			return nil, errors.New("redirected update directory")
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return close, nil
}

func pinPending(path string) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(p, windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		windows.CloseHandle(h)
		return nil, errors.New("invalid pending directory")
	}
	return os.NewFile(uintptr(h), path), nil
}

func validJournal(j upgradeJournal) error {
	if j.Format != 1 || len(j.Old) == 0 || len(j.New) != len(j.Old) {
		return errors.New("invalid upgrade journal")
	}
	for name, sum := range j.New {
		if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `\/:`) || strings.TrimRight(name, " .") != name || len(sum) != 64 || len(j.Old[name]) != 64 {
			return errors.New("invalid journal file")
		}
		if _, e := hex.DecodeString(sum); e != nil {
			return e
		}
		if _, e := hex.DecodeString(j.Old[name]); e != nil {
			return e
		}
	}
	return nil
}

func writeExclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}

// applyPayload retains exclusive file handles across verification and renames.
// All old files are quarantined (exe first); the new exe is activated last.
// A pending journal blocks the new app before it opens any user data.
func applyPayload(root, source string, j upgradeJournal, commit func() error, checkpoint func() error) error {
	if e := validJournal(j); e != nil {
		return e
	}
	var e error
	if root, e = safeDirectory(root); e != nil {
		return e
	}
	unpin, e := pinDirectories(root)
	if e != nil {
		return e
	}
	defer unpin()
	tick := func() error {
		if checkpoint != nil {
			return checkpoint()
		}
		return nil
	}
	pending := filepath.Join(root, ".planet-update")
	journalPath := filepath.Join(pending, "journal.json")
	bytes, e := json.Marshal(j)
	if e != nil {
		return e
	}
	exists := false
	var pendingHandle *os.File
	defer func() {
		if pendingHandle != nil {
			pendingHandle.Close()
		}
	}()
	if _, e = os.Lstat(pending); e == nil {
		exists = true
		if pendingHandle, e = pinPending(pending); e != nil {
			return e
		}
		got, e := os.ReadFile(journalPath)
		if e != nil {
			return errors.New("unrecognized pending update directory; left unchanged")
		}
		var saved upgradeJournal
		if e = json.Unmarshal(got, &saved); e != nil || !reflect.DeepEqual(saved, j) {
			return errors.New("pending update belongs to another package")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	// Validate every existing old/new/backup file before changing any of them.
	handles := map[string]*os.File{}
	defer func() {
		for _, f := range handles {
			f.Close()
		}
	}()
	names := make([]string, 0, len(j.Old))
	for n := range j.Old {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		backup := filepath.Join(pending, "old-"+name)
		oldBacked := false
		if exists {
			if _, e = os.Lstat(backup); e == nil {
				f, e := openOwned(backup, j.Old[name])
				if e != nil {
					return e
				}
				handles[backup] = f
				oldBacked = true
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		path := filepath.Join(root, name)
		want := j.Old[name]
		if oldBacked {
			want = j.New[name]
		}
		if _, e = os.Lstat(path); e == nil {
			f, e := openOwned(path, want)
			if e != nil {
				return e
			}
			handles[path] = f
		} else if !errors.Is(e, os.ErrNotExist) || !oldBacked {
			return fmt.Errorf("missing owned file: %s: %w", name, e)
		}
	}
	if !exists {
		// Publish a complete journal atomically. Interrupted preparation leaves
		// a separate, inactive directory, never a corrupt live journal.
		prepared, err := os.MkdirTemp(root, ".planet-update-preparing-")
		if err != nil {
			return err
		}
		pendingHandle, e = pinPending(prepared)
		if e != nil {
			return e
		}
		entries, err := pendingHandle.ReadDir(-1)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return errors.New("preparing directory is not empty; existing contents preserved")
		}
		if e = writeExclusive(filepath.Join(prepared, "journal.json"), bytes); e != nil {
			return e
		}
		if e = moveOwned(pendingHandle, pending); e != nil {
			return e
		}
		got, err := os.ReadFile(journalPath)
		if err != nil || string(got) != string(bytes) {
			return errors.New("pending journal changed during publication")
		}
	}
	if e = tick(); e != nil {
		return e
	}
	// Refill interrupted staging only from this package's verified source.
	for _, name := range names {
		staged := filepath.Join(pending, "new-"+name)
		if _, e = os.Lstat(staged); errors.Is(e, os.ErrNotExist) {
			// Already activated files need no stage copy.
			if handles[filepath.Join(pending, "old-"+name)] != nil && handles[filepath.Join(root, name)] != nil {
				continue
			}
			in, e := openOwned(filepath.Join(source, name), j.New[name])
			if e != nil {
				return e
			}
			in.Seek(0, io.SeekStart)
			out, e := os.CreateTemp(pending, ".partial-")
			partial := ""
			if e == nil {
				partial = out.Name()
			}
			if e == nil {
				_, e = io.Copy(out, in)
				if e == nil {
					e = out.Sync()
				}
				ce := out.Close()
				if e == nil {
					e = ce
				}
			}
			in.Close()
			if e != nil {
				return e
			}
			from, _ := windows.UTF16PtrFromString(partial)
			to, _ := windows.UTF16PtrFromString(staged)
			if e = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		f, e := openOwned(staged, j.New[name])
		if e != nil {
			return e
		}
		handles[staged] = f
	}
	if e = tick(); e != nil {
		return e
	}
	// Executable first. After it is removed, a crash cannot start the old app
	// with a partially switched dependency set.
	sort.Slice(names, func(a, b int) bool {
		if a == b {
			return false
		}
		if names[a] == "Planet Windows.exe" {
			return true
		}
		if names[b] == "Planet Windows.exe" {
			return false
		}
		return names[a] < names[b]
	})
	for _, name := range names {
		backup := filepath.Join(pending, "old-"+name)
		path := filepath.Join(root, name)
		if handles[backup] == nil {
			f := handles[path]
			if e = moveOwned(f, backup); e != nil {
				return e
			}
			handles[backup] = f
			delete(handles, path)
		}
		if e = tick(); e != nil {
			return e
		}
	}
	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		path := filepath.Join(root, name)
		if handles[path] == nil {
			staged := filepath.Join(pending, "new-"+name)
			f := handles[staged]
			if f == nil {
				return errors.New("staged update missing")
			}
			if e = moveOwned(f, path); e != nil {
				return e
			}
			handles[path] = f
			delete(handles, staged)
		}
		if e = tick(); e != nil {
			return e
		}
	}
	if e = commit(); e != nil {
		return e
	}
	if e = tick(); e != nil {
		return e
	}
	// Keep all recovery evidence; never recursively remove an installation path.
	// Atomic directory rename is the commit marker observed by the application.
	for path, f := range handles {
		f.Close()
		delete(handles, path)
	}
	archived, e := os.MkdirTemp(root, ".planet-update-completed-")
	if e != nil {
		return e
	}
	if e = os.Remove(archived); e != nil {
		return e
	}
	return moveOwned(pendingHandle, archived)
}

func readRegistryStrings(path string) (map[string]string, error) {
	k, e := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if e != nil {
		return nil, e
	}
	defer k.Close()
	names, e := k.ReadValueNames(0)
	if e != nil {
		return nil, e
	}
	values := map[string]string{}
	for _, name := range names {
		v, _, e := k.GetStringValue(name)
		if e != nil {
			return nil, e
		}
		values[name] = v
	}
	return values, nil
}
func setRegistryStrings(path string, values map[string]string) error {
	k, _, e := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	for name, value := range values {
		if e = k.SetStringValue(name, value); e != nil {
			return e
		}
	}
	return nil
}

func applyUpgrade(root, source string, files map[string]string) error {
	// Only the private embedded guard calls this operation. Its NSIS parent
	// holds the protocol mutex across staging, activation and process teardown.
	var e error
	root, e = safeDirectory(root)
	if e != nil {
		return e
	}
	var j upgradeJournal
	anchor, anchorErr := readRegistryStrings(updateKey)
	if anchorErr == nil && len(anchor) == 0 {
		// A process may stop between key creation and the atomic Journal value
		// write. No file operation can have started without that value.
		if _, err := os.Lstat(filepath.Join(root, ".planet-update")); !errors.Is(err, os.ErrNotExist) {
			return errors.New("empty transaction registration conflicts with pending files")
		}
		if e = checkUpgrade(root); e != nil {
			return e
		}
		state, err := readRegistryStrings(stateKey)
		if err != nil {
			return err
		}
		if state["TransactionProtocol"] != "2" {
			return errors.New("legacy installation cannot recover transaction")
		}
		if e = registry.DeleteKey(registry.CURRENT_USER, updateKey); e != nil {
			return e
		}
		anchorErr = registry.ErrNotExist
	}
	if anchorErr == nil {
		b := []byte(anchor["Journal"])
		if e = json.Unmarshal(b, &j); e != nil {
			return e
		}
		if j.Version != releaseVersion {
			return errors.New("resume with the same update package")
		}
	} else {
		if !errors.Is(anchorErr, registry.ErrNotExist) {
			return anchorErr
		}
		if e = checkUpgrade(root); e != nil {
			return e
		}
		state, e := readRegistryStrings(stateKey)
		if e != nil {
			return e
		}
		if state["TransactionProtocol"] != "2" {
			return errors.New("this legacy installer has no transaction lock; automatic migration is refused")
		}
		uninstall, e := readRegistryStrings(uninstallKey)
		if e != nil {
			return e
		}
		oldVersion := uninstall["DisplayVersion"]
		if !newerVersion(releaseVersion, oldVersion) {
			return errors.New("update must be a strictly newer release")
		}
		j = upgradeJournal{Format: 1, Version: releaseVersion, Old: map[string]string{}, New: map[string]string{}, State: state, Uninstall: uninstall}
		for name := range files {
			j.Old[name] = state[name]
			j.New[name] = files[name]
		}
		j.Old["Uninstall.exe"] = state["Uninstall.exe"]
		j.New["Uninstall.exe"], e = hashFile(filepath.Join(source, "Uninstall.exe"), true)
		if e != nil {
			return e
		}
		if len(state) != len(j.Old)+2 && len(state) != len(j.Old)+4 {
			return errors.New("unknown registered ownership state")
		}
	}
	if len(j.New) != len(files)+1 {
		return errors.New("journal payload differs")
	}
	for name, sum := range files {
		if j.New[name] != sum {
			return errors.New("resume with the same update package")
		}
	}
	sum, e := hashFile(filepath.Join(source, "Uninstall.exe"), true)
	if e != nil || sum != j.New["Uninstall.exe"] {
		return errors.New("uninstaller differs from pending update")
	}
	if !strings.EqualFold(j.State["InstallLocation"], root) || !strings.EqualFold(j.Uninstall["InstallLocation"], root) {
		return errors.New("journal location mismatch")
	}
	if e = validJournal(j); e != nil {
		return e
	}
	if anchorErr == nil {
		// Registry writes can be interrupted value by value, but recovery must
		// never adopt a removed registration or another installation's values.
		for key, original := range map[string]map[string]string{stateKey: j.State, uninstallKey: j.Uninstall} {
			current, err := readRegistryStrings(key)
			if err != nil {
				return err
			}
			if len(current) != len(original) {
				return errors.New("registration changed during pending update")
			}
			for name, value := range current {
				old, ok := original[name]
				if !ok {
					return errors.New("registration ownership changed")
				}
				newValue := old
				if key == stateKey {
					if next, ok := j.New[name]; ok {
						newValue = next
					}
				} else if name == "DisplayVersion" {
					newValue = j.Version
				}
				if value != old && value != newValue {
					return errors.New("registration changed during pending update")
				}
			}
		}
	}
	commit := func() error {
		state := map[string]string{}
		for n, v := range j.State {
			state[n] = v
		}
		for n, v := range j.New {
			state[n] = v
		}
		uninstall := map[string]string{}
		for n, v := range j.Uninstall {
			uninstall[n] = v
		}
		uninstall["DisplayVersion"] = releaseVersion
		if e := setRegistryStrings(stateKey, state); e != nil {
			return e
		}
		return setRegistryStrings(uninstallKey, uninstall)
	}
	// The registry, not a file supplied in a shared directory, authorizes
	// journal recovery. A single value is published before any file moves.
	if anchorErr != nil {
		b, _ := json.Marshal(j)
		k, existing, err := registry.CreateKey(registry.CURRENT_USER, updateKey, registry.SET_VALUE)
		if err != nil {
			return err
		}
		if existing {
			k.Close()
			return errors.New("transaction was concurrently registered")
		}
		err = k.SetStringValue("Journal", string(b))
		k.Close()
		if err != nil {
			registry.DeleteKey(registry.CURRENT_USER, updateKey)
			return err
		}
	}
	// Crash after archive but before removing the anchor: verify the committed
	// payload and finish registration rather than trying to quarantine it again.
	if anchorErr == nil {
		if _, err := os.Lstat(filepath.Join(root, ".planet-update")); errors.Is(err, os.ErrNotExist) {
			allNew := true
			for n, v := range j.New {
				got, err := hashFile(filepath.Join(root, n), true)
				if err != nil || got != v {
					allNew = false
					break
				}
			}
			if allNew {
				if e = commit(); e != nil {
					return e
				}
				return registry.DeleteKey(registry.CURRENT_USER, updateKey)
			}
		}
	}
	if e = applyPayload(root, source, j, commit, nil); e != nil {
		return e
	}
	return registry.DeleteKey(registry.CURRENT_USER, updateKey)
}

func newerVersion(newVersion, oldVersion string) bool {
	parse := func(s string) ([3]int, bool) {
		var v [3]int
		var tail string
		n, _ := fmt.Sscanf(s, "%d.%d.%d%s", &v[0], &v[1], &v[2], &tail)
		return v, n == 3 && v[0] >= 0 && v[1] >= 0 && v[2] >= 0 && fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) == s
	}
	a, ok := parse(newVersion)
	if !ok {
		return false
	}
	b, ok := parse(oldVersion)
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
