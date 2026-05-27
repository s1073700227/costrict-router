//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	windowsDetachedProcess = 0x00000008
	windowsCreateNoWindow  = 0x08000000
)

func detachProcess(cmd *exec.Cmd) {
	// DETACHED_PROCESS 让后台服务脱离当前控制台，避免关闭终端时带停 serve 子进程。
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windowsDetachedProcess | windowsCreateNoWindow,
		HideWindow:    true,
	}
}
