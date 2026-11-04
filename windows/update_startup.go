package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func checkUpdateReady(executable string) error {
	_, err := os.Lstat(filepath.Join(filepath.Dir(executable), ".planet-update"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("检测到未完成的程序升级。请关闭 Planet，重新运行刚才的同一安装包以恢复升级；资料库尚未打开")
}
