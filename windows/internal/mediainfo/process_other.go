//go:build !windows

package mediainfo

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
