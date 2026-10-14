package controller

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

const updaterTreeKillDelay = 5 * time.Second

// commandLineWriter lets Cmd own pipe-copy cancellation and keeps partial lines bounded.
type commandLineWriter struct {
	pending []byte
	emit    func(string)
}

func (w *commandLineWriter) Write(p []byte) (int, error) {
	count := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		if end < 0 {
			end = len(p) - 1
		}
		take := end + 1
		w.pending = append(w.pending, p[:take]...)
		p = p[take:]
		if w.pending[len(w.pending)-1] == '\n' || len(w.pending) >= 64*1024 {
			w.flush()
		}
	}
	return count, nil
}
func (w *commandLineWriter) flush() {
	if len(w.pending) > 0 {
		w.emit(string(w.pending))
		w.pending = w.pending[:0]
	}
}

func runDirectCommandTree(ctx context.Context, command agents.Command, output func(string)) error {
	argv := command.Args()
	if len(argv) == 0 {
		return errors.New("runtime update command is empty")
	}
	if err := managedruntime.PrepareNPMProjectPrefix(argv); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = filteredInstallEnv()
	stdout, stderr := &commandLineWriter{emit: output}, &commandLineWriter{emit: output}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = updaterTreeKillDelay + time.Second
	stop := configureUpdaterTree(cmd)
	defer stop()
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	stdout.flush()
	stderr.flush()
	if ctx.Err() == nil && errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}
