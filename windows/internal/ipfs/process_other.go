//go:build !windows

package ipfs

import "os/exec"

func hideWindow(_ *exec.Cmd) {}
