//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	// Setsid 创建新的 session，避免终端关闭时通过控制终端影响后台服务。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
