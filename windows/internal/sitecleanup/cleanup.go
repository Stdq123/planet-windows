// Package sitecleanup removes only site-owned directories after deletion intent is durable.
package sitecleanup

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var siteIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var loopbackEndpoint = regexp.MustCompile(`^/ip4/127\.0\.0\.1/tcp/([0-9]{1,5})$`)

func ValidSiteID(id string) bool { return siteIDPattern.MatchString(id) }

func CheckOfflineKuboConfig(root string) error {
	return CheckOfflineKuboRepo(filepath.Join(root, "ipfs"))
}

func CheckOfflineKuboRepo(repo string) error {
	path := filepath.Join(repo, "config")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return errors.New("Kubo 配置无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return errors.New("Kubo 配置已改变")
	}
	b, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return errors.New("Kubo 配置无效")
	}
	var config struct {
		Addresses struct {
			API     string `json:"API"`
			Gateway string `json:"Gateway"`
		} `json:"Addresses"`
	}
	if json.Unmarshal(b, &config) != nil {
		return errors.New("Kubo 配置无效")
	}
	for _, address := range []string{config.Addresses.API, config.Addresses.Gateway} {
		match := loopbackEndpoint.FindStringSubmatch(address)
		if len(match) != 2 {
			return errors.New("Kubo API 或网关不是本机地址，无法安全启动")
		}
		port, err := strconv.Atoi(match[1])
		if err != nil || port > 65535 {
			return errors.New("Kubo 端口无效")
		}
	}
	return nil
}

// KuboRepositoryPresent refuses links and incomplete repositories rather than
// creating a new one while trying to remove an existing private key.
func KuboRepositoryPresent(root string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(resolved, "ipfs")
	return KuboRepoPresent(dir)
}

func KuboRepoPresent(dir string) (bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("Kubo 目录无效")
	}
	info, err = os.Lstat(filepath.Join(dir, "config"))
	if err != nil || !info.Mode().IsRegular() {
		return false, errors.New("Kubo 仓库不完整，无法核对密钥")
	}
	keystore := filepath.Join(dir, "keystore")
	info, err = os.Lstat(keystore)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("Kubo 密钥目录无效或被重定向")
	}
	actual, err := filepath.EvalSymlinks(keystore)
	if err != nil {
		return false, err
	}
	actual, err = filepath.Abs(actual)
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(keystore)) {
		return false, errors.New("Kubo 密钥目录被重定向")
	}
	if err = filepath.WalkDir(keystore, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, e := os.Lstat(path)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("Kubo 密钥目录含链接或特殊文件")
		}
		return nil
	}); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveOwnedDirectory only removes a regular, link-free tree below an allowed
// app-owned parent. Shared Kubo blocks and template resources are not touched.
func RemoveOwnedDirectory(root, kind, siteID string) error {
	if !ValidSiteID(siteID) || (kind != "attachments" && kind != "draft-attachments" && kind != "imports") {
		return errors.New("站点清理路径无效")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	parent := filepath.Join(resolved, kind)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("站点清理目录无效")
	}
	target := filepath.Join(parent, siteID)
	relative, err := filepath.Rel(parent, target)
	if err != nil || relative != siteID || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("站点清理路径越界")
	}
	info, err = os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("站点目录无效")
	}
	if err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("站点目录含不支持的链接或特殊文件")
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(target)
}
