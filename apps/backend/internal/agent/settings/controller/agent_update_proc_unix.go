//go:build !windows

package controller

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func configureUpdaterTree(cmd *exec.Cmd) func() {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var mu sync.Mutex
	var timer *time.Timer
	stopped := false
	cmd.Cancel = func() error {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return os.ErrProcessDone
		}
		pid := cmd.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			err = os.ErrProcessDone
		}
		timer = time.AfterFunc(updaterTreeKillDelay, func() {
			mu.Lock()
			defer mu.Unlock()
			if !stopped {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		})
		return err
	}
	return func() {
		mu.Lock()
		defer mu.Unlock()
		stopped = true
		if timer != nil {
			timer.Stop()
		}
	}
}
