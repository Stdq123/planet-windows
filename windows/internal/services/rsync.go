package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var rsyncDestinationPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}@[A-Za-z0-9][A-Za-z0-9.-]{0,252}:/[A-Za-z0-9_./-]+$`)

func ValidRsyncDestination(value string) bool {
	if !rsyncDestinationPattern.MatchString(value) {
		return false
	}
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 || parts[1] == "/" || strings.Contains(parts[1], "//") {
		return false
	}
	for _, part := range strings.Split(parts[1], "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func rsyncArgs(source, destination, keyPath string, deleteRemote bool) ([]string, error) {
	if !ValidRsyncDestination(destination) {
		return nil, errors.New("SSH 目标须为 user@host:/目录，且不能使用根目录或特殊字符")
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	slash := filepath.ToSlash(abs)
	volume := filepath.VolumeName(abs)
	if len(volume) == 2 && volume[1] == ':' {
		slash = "/cygdrive/" + strings.ToLower(volume[:1]) + strings.TrimPrefix(slash, filepath.ToSlash(volume))
	}
	if !strings.HasSuffix(slash, "/") {
		slash += "/"
	}
	ssh := "ssh -oBatchMode=yes -oStrictHostKeyChecking=yes"
	if keyPath != "" {
		if !filepath.IsAbs(keyPath) || strings.ContainsAny(keyPath, "\"'`$;&|\r\n") {
			return nil, errors.New("SSH 私钥路径无效")
		}
		ssh += ` -i "` + filepath.ToSlash(keyPath) + `"`
	}
	args := []string{"--archive", "--protect-args"}
	if deleteRemote {
		args = append(args, "--delete")
	}
	args = append(args, "-e", ssh, "--", slash, destination)
	return args, nil
}

// DeployRsync uses an installed Cygwin-compatible rsync and Windows OpenSSH.
func DeployRsync(source, destination, keyPath string, deleteRemote bool) error {
	args, err := rsyncArgs(source, destination, keyPath, deleteRemote)
	if err != nil {
		return err
	}
	if keyPath != "" {
		info, e := os.Stat(keyPath)
		if e != nil || !info.Mode().IsRegular() {
			return errors.New("SSH 私钥文件不存在或无效")
		}
	}
	exe, err := exec.LookPath("rsync")
	if err != nil {
		return errors.New("未找到 rsync，请先安装支持 Cygwin 路径的 rsync 并加入 PATH")
	}
	if _, err = exec.LookPath("ssh"); err != nil {
		return errors.New("未找到 OpenSSH 客户端")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	hideWindow(cmd)
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("SSH 发布超时")
		}
		return fmt.Errorf("SSH rsync 发布失败，请检查目标、主机密钥和 SSH 凭据：%w", err)
	}
	return nil
}
