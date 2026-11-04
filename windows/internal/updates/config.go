package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func TrustIdentity(cfg Config) string {
	h := sha256.Sum256([]byte(cfg.FeedURL + "\n" + cfg.PublicKey))
	return hex.EncodeToString(h[:])
}
func LoadConfig(root string) (Config, error) {
	path := filepath.Join(root, "update-settings.json")
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return Config{}, nil
	}
	if e != nil {
		return Config{}, e
	}
	if !info.Mode().IsRegular() {
		return Config{}, errors.New("更新设置不能是链接或目录")
	}
	f, e := os.Open(path)
	if e != nil {
		return Config{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 16385))
	if e != nil || len(b) > 16384 {
		return Config{}, errors.New("更新设置文件无效")
	}
	var cfg Config
	if e = strictJSON(b, &cfg); e != nil {
		return Config{}, errors.New("无法读取更新设置")
	}
	return ValidateConfig(cfg)
}
func saveConfig(root string, cfg Config) error {
	if e := ensureDirectory(root); e != nil {
		return e
	}
	path := filepath.Join(root, "update-settings.json")
	if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
		return errors.New("更新设置路径不能是链接")
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	b, e := json.MarshalIndent(cfg, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(root, ".update-settings-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func SaveSettings(root string, input Config) (Config, error) {
	// Callers never supply replay-protection state; it belongs to this identity.
	input.HighestVersion = ""
	input.HighestSHA256 = ""
	input, e := ValidateConfig(input)
	if e != nil {
		return Config{}, e
	}
	current, e := LoadConfig(root)
	if e != nil {
		return Config{}, e
	}
	if TrustIdentity(input) == TrustIdentity(current) {
		input.HighestVersion = current.HighestVersion
		input.HighestSHA256 = current.HighestSHA256
	}
	if e = saveConfig(root, input); e != nil {
		return Config{}, e
	}
	return input, nil
}
func RecordSeen(root string, identity Config, r Release) error {
	current, e := LoadConfig(root)
	if e != nil {
		return e
	}
	if TrustIdentity(current) != TrustIdentity(identity) {
		return errors.New("更新源或公钥已改变，请重新检查")
	}
	if current.HighestVersion != "" {
		cmp, e := Compare(r.Version, current.HighestVersion)
		if e != nil || cmp < 0 || (cmp == 0 && current.HighestSHA256 != r.SHA256) {
			return errors.New("更新版本记录不允许回退或换包")
		}
	}
	current.HighestVersion = r.Version
	current.HighestSHA256 = r.SHA256
	return saveConfig(root, current)
}

// NewDownloadDirectory keeps unsigned partial bytes inside private app data.
func NewDownloadDirectory(root string) (string, error) {
	path := filepath.Join(root, "updates")
	if e := ensureDirectory(path); e != nil {
		return "", e
	}
	return os.MkdirTemp(path, "download-")
}

func VerifyDownloaded(path string, r Release) error {
	// The exclusive handle blocks mutation during the final on-disk check.
	f, e := openDownloaded(path)
	if e != nil {
		return e
	}
	defer f.Close()
	sum := sha256.New()
	n, e := io.Copy(sum, io.LimitReader(f, r.Size+1))
	if e != nil {
		return e
	}
	if n != r.Size || hex.EncodeToString(sum.Sum(nil)) != r.SHA256 {
		return errors.New("已保存的更新包被修改，请重新下载")
	}
	return nil
}

// Remove only the exact, unlinked file this download produced. Changed files
// remain for inspection instead of being treated as disposable user content.
func RemoveVerifiedDownload(path string, r Release) error {
	f, e := openDownloadForAccess(path, true)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, r.Size+1))
	if e != nil {
		return e
	}
	if n != r.Size || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return errors.New("更新缓存已变化，已保留文件")
	}
	return discardDownload(f)
}
