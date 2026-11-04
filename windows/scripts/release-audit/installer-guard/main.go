//go:build windows

// installer-guard is embedded in the independent NSIS installer and its
// uninstaller. It never executes the installed app or modifies user content.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var manifestBase64 string // set by -ldflags from the actual build payload

var stateKey = `Software\PlanetWindows\InstallerState`
var uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\org.planetable.planetwindows`

func manifest() (map[string]string, error) {
	b, err := base64.StdEncoding.DecodeString(manifestBase64)
	if err != nil {
		return nil, err
	}
	var files map[string]string
	if err := json.Unmarshal(b, &files); err != nil || len(files) == 0 {
		return nil, errors.New("invalid embedded payload manifest")
	}
	for name, sum := range files {
		if name == "" || filepath.Base(name) != name || len(sum) != 64 {
			return nil, errors.New("invalid payload manifest entry")
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return nil, errors.New("invalid payload hash")
		}
	}
	return files, nil
}

func safeDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || !filepath.IsAbs(absolute) {
		return "", errors.New("invalid installation path")
	}
	absolute = filepath.Clean(absolute)
	if filepath.Dir(absolute) == absolute {
		return "", errors.New("a drive root cannot be an installation directory")
	}
	for current := absolute; ; current = filepath.Dir(current) {
		p, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return "", err
		}
		attrs, err := windows.GetFileAttributes(p)
		if err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return "", fmt.Errorf("reparse point in installation path: %s", current)
		}
		if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return absolute, nil
}

func existsKey(path string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	k.Close()
	return true, nil
}

func checkInstall(path string) error {
	path, err := safeDirectory(path)
	if err != nil {
		return err
	}
	for _, key := range []string{stateKey, uninstallKey, updateKey} {
		exists, err := existsKey(key)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("an existing Planet Windows installation is registered; automatic upgrade is not supported")
		}
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("installation directory must be empty; existing files were not changed")
	}
	return nil
}

func hashFile(path string, exclusive bool) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("owned path is not a regular file")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return "", err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", errors.New("owned path is a reparse point")
	}
	var f *os.File
	if exclusive {
		h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return "", fmt.Errorf("file is locked or inaccessible: %s: %w", path, err)
		}
		var fileInfo windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &fileInfo); err != nil {
			windows.CloseHandle(h)
			return "", err
		}
		if fileInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			windows.CloseHandle(h)
			return "", errors.New("owned path is a reparse point")
		}
		f = os.NewFile(uintptr(h), path)
		if f == nil {
			windows.CloseHandle(h)
			return "", errors.New("cannot wrap file handle")
		}
	} else {
		f, err = os.Open(path)
		if err != nil {
			return "", err
		}
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func record(path string, expected map[string]string) (retErr error) {
	path, err := safeDirectory(path)
	if err != nil {
		return err
	}
	exists, err := existsKey(stateKey)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("installer state already exists")
	}
	hashes := make(map[string]string, len(expected)+1)
	for name, want := range expected {
		got, err := hashFile(filepath.Join(path, name), false)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("installed file differs from package: %s", name)
		}
		hashes[name] = got
	}
	hashes["Uninstall.exe"], err = hashFile(filepath.Join(path, "Uninstall.exe"), false)
	if err != nil {
		return err
	}
	k, openedExisting, err := registry.CreateKey(registry.CURRENT_USER, stateKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() {
		_ = k.Close()
		if retErr != nil && !openedExisting {
			if cleanupErr := registry.DeleteKey(registry.CURRENT_USER, stateKey); cleanupErr != nil {
				retErr = fmt.Errorf("%v; could not remove incomplete installer state: %w", retErr, cleanupErr)
			}
		}
	}()
	if openedExisting {
		return errors.New("installer state was concurrently created")
	}
	if err := k.SetStringValue("InstallLocation", path); err != nil {
		return err
	}
	if err := k.SetStringValue("TransactionProtocol", "2"); err != nil {
		return err
	}
	for name, sum := range hashes {
		if err := k.SetStringValue(name, sum); err != nil {
			return err
		}
	}
	return nil
}

func checkUninstall(path string, expected map[string]string) error {
	if exists, err := existsKey(updateKey); err != nil {
		return err
	} else if exists {
		return errors.New("finish the pending transaction before uninstalling")
	}
	if _, err := os.Lstat(filepath.Join(path, ".planet-update")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("finish the pending update before uninstalling")
	}
	path, err := safeDirectory(path)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.QUERY_VALUE)
	if err != nil {
		return errors.New("trusted installer state not found; refusing automatic removal")
	}
	defer k.Close()
	location, _, err := k.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(location), path) {
		return errors.New("registered installation path does not match")
	}
	if protocol, _, _ := k.GetStringValue("TransactionProtocol"); protocol == "2" {
		entry, e := readRegistryStrings(uninstallKey)
		if e != nil {
			return e
		}
		if entry["DisplayVersion"] != releaseVersion {
			return errors.New("use the current version's uninstaller")
		}
	}
	names := make([]string, 0, len(expected)+1)
	for name := range expected {
		names = append(names, name)
	}
	names = append(names, "Uninstall.exe")
	for _, name := range names {
		want, _, err := k.GetStringValue(name)
		if err != nil || len(want) != 64 {
			return fmt.Errorf("missing trusted hash for %s", name)
		}
		if packaged, ok := expected[name]; ok && !strings.EqualFold(packaged, want) {
			return errors.New("uninstaller belongs to a different payload")
		}
		got, err := hashFile(filepath.Join(path, name), true)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("file changed since installation: %s", name)
		}
	}
	return nil
}

// checkUpgrade is a read-only preflight, not a transaction or authorization
// to overwrite anything. A future commit operation must revalidate while
// holding the necessary handles, because paths can change after this returns.
func checkUpgrade(path string) error {
	path, err := safeDirectory(path)
	if err != nil {
		return err
	}
	uninstall, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return errors.New("registered safe installation not found")
	}
	location, _, err := uninstall.GetStringValue("InstallLocation")
	uninstall.Close()
	if err != nil || !strings.EqualFold(filepath.Clean(location), path) {
		return errors.New("uninstall registration does not match installation path")
	}
	state, err := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.QUERY_VALUE)
	if err != nil {
		return errors.New("trusted installer state not found")
	}
	defer state.Close()
	location, _, err = state.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(location), path) {
		return errors.New("installer state does not match installation path")
	}
	// Generation 1 recorded individual values without storing a manifest.
	// Require that complete known set even when a future payload differs.
	required := map[string]bool{
		"planet windows.exe": false, "ipfs.exe": false, "icon.png": false,
		"kubo-license.txt": false, "kubo-license-mit.txt": false, "kubo-license-apache.txt": false,
		"license-planet.txt": false, "third-party-notices.txt": false,
		"kubo-buildinfo.txt": false, "kubo-dependency-licenses.txt": false,
		"uninstall.exe": false,
	}
	values, err := state.ReadValueNames(0)
	if err != nil {
		return err
	}
	var names []string
	for _, name := range values {
		lower := strings.ToLower(name)
		if lower == "installlocation" || lower == "shortcutpath" || lower == "shortcutsha256" || lower == "transactionprotocol" {
			continue
		}
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `\/:`) || strings.TrimRight(name, " .") != name {
			return fmt.Errorf("invalid registered file name %q", name)
		}
		if _, ok := required[lower]; ok {
			required[lower] = true
		}
		names = append(names, name)
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("missing registered hash for %s", name)
		}
	}
	for _, name := range names {
		want, _, err := state.GetStringValue(name)
		if err != nil || len(want) != 64 {
			return fmt.Errorf("missing trusted hash for %s", name)
		}
		if _, err := hex.DecodeString(want); err != nil {
			return fmt.Errorf("invalid trusted hash for %s", name)
		}
		got, err := hashFile(filepath.Join(path, name), true)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("file changed since installation: %s", name)
		}
	}
	return nil
}

// checkIdenticalInstall permits a strictly read-only no-op when the installed
// payload matches this package. It does not permit repairs or version upgrades.
func checkIdenticalInstall(path string, expected map[string]string) error {
	if _, err := os.Lstat(filepath.Join(path, ".planet-update")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("pending update requires recovery")
	}
	if pending, err := existsKey(updateKey); err != nil {
		return err
	} else if pending {
		return errors.New("pending transaction requires recovery")
	}
	if err := checkUpgrade(path); err != nil {
		return err
	}
	entry, err := readRegistryStrings(uninstallKey)
	if err != nil {
		return err
	}
	if entry["DisplayVersion"] != releaseVersion {
		return errors.New("release version differs; run the upgrade transaction")
	}
	state, err := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer state.Close()
	expectedNames := make(map[string]string, len(expected))
	for name, hash := range expected {
		expectedNames[strings.ToLower(name)] = hash
	}
	values, err := state.ReadValueNames(0)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(expected))
	for _, name := range values {
		lower := strings.ToLower(name)
		if lower == "installlocation" || lower == "shortcutpath" || lower == "shortcutsha256" || lower == "uninstall.exe" || lower == "transactionprotocol" {
			continue
		}
		want, ok := expectedNames[lower]
		if !ok {
			return fmt.Errorf("registered payload differs from this package: %s", name)
		}
		got, _, err := state.GetStringValue(name)
		if err != nil || !strings.EqualFold(got, want) {
			return fmt.Errorf("registered payload differs from this package: %s", name)
		}
		seen[lower] = true
	}
	if len(seen) != len(expectedNames) {
		return errors.New("registered payload differs from this package")
	}
	return nil
}

// createShortcut copies into a private temporary file in the destination
// directory, then uses MoveFile (which never replaces an existing target).
func createShortcut(installDir, source, destination string) error {
	installDir, err := safeDirectory(installDir)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(destination), "Planet Windows.lnk") {
		return errors.New("unexpected shortcut name")
	}
	directory, err := safeDirectory(filepath.Dir(destination))
	if err != nil {
		return err
	}
	destination = filepath.Join(directory, filepath.Base(destination))
	k, err := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	location, _, err := k.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(location), installDir) {
		return errors.New("registered installation path does not match")
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	temp, err := os.CreateTemp(directory, ".planet-shortcut-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = io.Copy(temp, sourceFile); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	want, err := hashFile(tempPath, true)
	if err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	if err = windows.MoveFile(from, to); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return nil
		}
		return err
	}
	got, err := hashFile(destination, true)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return errors.New("shortcut changed during creation; it was not registered")
	}
	if err := k.SetStringValue("ShortcutSHA256", want); err != nil {
		return err
	}
	return k.SetStringValue("ShortcutPath", destination)
}

// removeMatchingFile holds an exclusive handle through hashing and deletion,
// so a different file cannot replace the one being deleted in between.
func removeMatchingFile(path, want string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|0x00010000, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		windows.CloseHandle(h)
		return errors.New("cannot wrap shortcut handle")
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), want) {
		return nil
	}
	flag := byte(1)
	return windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &flag, 1)
}

func removeShortcut(installDir, destination string) error {
	installDir, err := safeDirectory(installDir)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(destination), "Planet Windows.lnk") {
		return errors.New("unexpected shortcut name")
	}
	directory, err := safeDirectory(filepath.Dir(destination))
	if err != nil {
		return nil
	} // preserve if redirected
	destination = filepath.Join(directory, filepath.Base(destination))
	k, err := registry.OpenKey(registry.CURRENT_USER, stateKey, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	location, _, err := k.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(location), installDir) {
		return errors.New("registered installation path does not match")
	}
	registered, _, err := k.GetStringValue("ShortcutPath")
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	want, _, err := k.GetStringValue("ShortcutSHA256")
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(registered), destination) || len(want) != 64 {
		return nil
	}
	return removeMatchingFile(destination, want)
}

func run() error {
	if len(os.Args) < 3 {
		return errors.New("expected operation and installation directory")
	}
	files, err := manifest()
	if err != nil {
		return err
	}
	if os.Args[1] == "upgrade-apply" {
		if len(os.Args) != 6 {
			return errors.New("expected NSIS parent identity and mutex handle")
		}
		pid, e := strconv.ParseUint(os.Args[4], 10, 32)
		if e != nil {
			return e
		}
		value, e := parseMutexHandle(os.Args[5])
		if e != nil {
			return e
		}
		h, e := retainInstallerLock(uint32(pid), value)
		if e != nil {
			return e
		}
		defer windows.CloseHandle(h)
	}
	switch os.Args[1] {
	case "install-check":
		return checkInstall(os.Args[2])
	case "record":
		return record(os.Args[2], files)
	case "uninstall-check":
		return checkUninstall(os.Args[2], files)
	case "upgrade-check":
		return checkUpgrade(os.Args[2])
	case "upgrade-apply":
		if len(os.Args) != 6 {
			return errors.New("expected staged payload directory")
		}
		return applyUpgrade(os.Args[2], os.Args[3], files)
	case "identical-install-check":
		return checkIdenticalInstall(os.Args[2], files)
	case "shortcut-create":
		if len(os.Args) != 5 {
			return errors.New("expected shortcut source and destination")
		}
		return createShortcut(os.Args[2], os.Args[3], os.Args[4])
	case "shortcut-remove":
		if len(os.Args) != 4 {
			return errors.New("expected shortcut destination")
		}
		return removeShortcut(os.Args[2], os.Args[3])
	default:
		return errors.New("unknown installer guard operation")
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
