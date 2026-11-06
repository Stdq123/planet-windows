package services

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Name         string        `json:"name"`
	Token        string        `json:"token"`
	Server       string        `json:"server,omitempty"`
	LastCID      string        `json:"lastCid,omitempty"`
	RequestID    string        `json:"requestId,omitempty"`
	PendingCID   string        `json:"pendingCid,omitempty"`
	DeleteRemote bool          `json:"deleteRemote,omitempty"`
	AutoDeploy   bool          `json:"autoDeploy,omitempty"`
	LastURL      string        `json:"lastUrl,omitempty"`
	XCredentials *XCredentials `json:"xCredentials,omitempty"`
	AutoSync     bool          `json:"autoSync,omitempty"`
	XEnabledAt   time.Time     `json:"xEnabledAt,omitempty"`
	XError       string        `json:"xError,omitempty"`
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9-]{1,100}$`)

func configPath(root, id, kind string, create bool) (string, error) {
	if !safeID.MatchString(id) || (kind != "filebase" && kind != "plausible" && kind != "cloudflare" && kind != "ssh" && kind != "x" && kind != "x-post" && kind != "telegram") {
		return "", errors.New("服务配置标识无效")
	}
	// Check ancestors directly: EvalSymlinks also expands Windows short names,
	// so comparing its result with the input would reject ordinary directories.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for ancestor := absRoot; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("服务配置目录不可重定向")
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	dir := root
	for _, part := range []string{"integrations", kind} {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(dir, 0700); err != nil {
				return "", err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("服务配置目录无效")
		}
	}
	return filepath.Join(dir, id+".dat"), nil
}
func SaveConfig(root, id, kind string, cfg Config) error {
	if kind == "telegram" {
		return errors.New("Telegram 配置必须使用专用接口")
	}
	if kind == "x" && (cfg.Token != "" || cfg.XCredentials != nil) && !validXConfig(cfg) {
		return errors.New("X 凭据配置无效")
	}
	if cfg.Token != "" && kind == "x-post" && !validXPostConfig(id, cfg) {
		return errors.New("X 同步记录无效")
	}
	if cfg.Token != "" && kind == "cloudflare" && !ValidCloudflareConfig(cfg.Server, cfg.Name, cfg.Token) {
		return errors.New("Cloudflare 配置无效")
	}
	if cfg.Token != "" && kind == "ssh" {
		if cfg.Token != "ssh-agent" || !ValidRsyncDestination(cfg.Name) {
			return errors.New("SSH 配置无效")
		}
		if cfg.Server != "" && (!filepath.IsAbs(cfg.Server) || strings.ContainsAny(cfg.Server, "\"'`$;&|\r\n")) {
			return errors.New("SSH 私钥路径无效")
		}
	}
	if kind == "plausible" && cfg.Server != "" {
		origin, err := PlausibleServerOrigin(cfg.Server)
		if err != nil {
			return err
		}
		cfg.Server = origin
	}
	if cfg.Token != "" && (!validToken(cfg.Token) || strings.TrimSpace(cfg.Name) == "" || len(cfg.Name) > 255) {
		return errors.New("服务配置无效")
	}
	path, err := configPath(root, id, kind, cfg.Token != "")
	if errors.Is(err, os.ErrNotExist) && cfg.Token == "" {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("服务配置文件无效")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if cfg.Token == "" {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return os.Remove(path)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	sealed, err := protect(raw)
	if err != nil {
		return errors.New("无法保护服务配置")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func LoadConfig(root, id, kind string) (Config, error) {
	if kind == "telegram" {
		return Config{}, errors.New("Telegram 配置必须使用专用接口")
	}
	path, err := configPath(root, id, kind, false)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return Config{}, errors.New("服务配置文件无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return Config{}, errors.New("服务配置读取失败")
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return Config{}, errors.New("服务配置读取失败")
	}
	raw, err := unprotect(data)
	if err != nil {
		return Config{}, errors.New("无法解锁服务配置")
	}
	var cfg Config
	if json.Unmarshal(raw, &cfg) != nil || !validToken(cfg.Token) || strings.TrimSpace(cfg.Name) == "" || len(cfg.Name) > 255 {
		return Config{}, errors.New("服务配置无效")
	}
	if kind == "cloudflare" && !ValidCloudflareConfig(cfg.Server, cfg.Name, cfg.Token) {
		return Config{}, errors.New("Cloudflare 配置无效")
	}
	if kind == "x" && !validXConfig(cfg) {
		return Config{}, errors.New("X 凭据配置无效")
	}
	if kind == "x-post" && !validXPostConfig(id, cfg) {
		return Config{}, errors.New("X 同步记录无效")
	}
	if kind == "ssh" && (cfg.Token != "ssh-agent" || !ValidRsyncDestination(cfg.Name) || cfg.Server != "" && (!filepath.IsAbs(cfg.Server) || strings.ContainsAny(cfg.Server, "\"'`$;&|\r\n"))) {
		return Config{}, errors.New("SSH 配置无效")
	}
	if kind == "plausible" && cfg.Server != "" {
		origin, err := PlausibleServerOrigin(cfg.Server)
		if err != nil {
			return Config{}, err
		}
		cfg.Server = origin
	}
	return cfg, nil
}
