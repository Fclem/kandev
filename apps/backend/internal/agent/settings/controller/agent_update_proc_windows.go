//go:build windows

package controller

import "os/exec"

// CommandContext kills the updater only; WaitDelay bounds inherited pipes.
func configureUpdaterTree(_ *exec.Cmd) func() { return func() {} }
