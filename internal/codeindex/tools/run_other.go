//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package tools

import "os/exec"

func configureProcess(cmd *exec.Cmd) func() { return func() {} }
