package services

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	Name       string `json:"name"`
	Token      string `json:"token"`
	Server     string `json:"server,omitempty"`
	LastCID    string `json:"lastCid,omitempty"`
	RequestID  string `json:"requestId,omitempty"`
	PendingCID string `json:"pendingCid,omitempty"`
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9-]{1,100}$`)

func configPath(root, id, kind string, create bool) (string, error) {
	if !safeID.MatchString(id) || (kind != "filebase" && kind != "plausible") {
		return "", errors.New("服务配置标识无效")
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
		resolved, e := filepath.EvalSymlinks(dir)
		if e != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(dir)) {
			return "", errors.New("服务配置目录不可重定向")
		}
	}
	return filepath.Join(dir, id+".dat"), nil
}
func SaveConfig(root, id, kind string, cfg Config) error {
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
	if kind == "plausible" && cfg.Server != "" {
		origin, err := PlausibleServerOrigin(cfg.Server)
		if err != nil {
			return Config{}, err
		}
		cfg.Server = origin
	}
	return cfg, nil
}
