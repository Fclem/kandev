package process

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

const (
	catalogHelperStderrLimit = 64 << 10
	catalogHelperReapTimeout = sessioncatalog.TaskProbeHelperReapTimeout
	catalogHelperWaitDelay   = sessioncatalog.TaskProbeHelperWaitDelay
)

var errCatalogHelperUnavailable = errors.New("catalog helper launch context is unavailable")

// catalogHelper is one fresh observation process. It owns private pipes and
// its own process group; it is never attached to the live adapter, journal,
// transcript or execution slot.
type catalogHelper struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *boundedCatalogStderr
	lifecycle processLifecycleHandle
}

// boundedCatalogStderr drains provider diagnostics without retaining more
// than a fixed prefix. Its content never leaves agentctl.
type boundedCatalogStderr struct {
	mu    sync.Mutex
	bytes int
}

func (b *boundedCatalogStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.bytes = min(b.bytes+len(p), catalogHelperStderrLimit)
	b.mu.Unlock()
	return len(p), nil
}

func (b *boundedCatalogStderr) size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.bytes
}

// startCatalogHelper launches the sealed command again. An unavailable
// executable or working directory is refused; another binary is never chosen.
func startCatalogHelper(launch *catalogLaunch) (*catalogHelper, error) {
	if launch == nil || !catalogLaunchAvailable(launch) {
		return nil, errCatalogHelperUnavailable
	}
	cmd := launch.helperCommand()
	setAgentProcGroup(cmd)
	cmd.WaitDelay = catalogHelperWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errCatalogHelperUnavailable
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, errCatalogHelperUnavailable
	}
	stderr := &boundedCatalogStderr{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, errCatalogHelperUnavailable
	}
	lifecycle, err := installProcessLifecycle(cmd)
	if err != nil {
		reapErr := killAndWaitStartedCommand(cmd)
		return nil, errors.Join(errCatalogHelperUnavailable, reapErr)
	}
	return &catalogHelper{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr, lifecycle: lifecycle}, nil
}

func catalogLaunchAvailable(launch *catalogLaunch) bool {
	dir, err := os.Stat(launch.dir)
	if launch.dir != "" && (err != nil || !dir.IsDir()) {
		return false
	}
	executable, err := os.Stat(launch.path)
	return err == nil && executable.Mode().IsRegular()
}

// stop kills and reaps only the helper's own process group, then releases
// its pipes. The live agent process is never signalled.
func (h *catalogHelper) stop() error {
	_ = h.stdin.Close()
	if ownsProcessLifecycle(h.lifecycle) {
		_ = reapProcessLifecycle(h.lifecycle)
	} else {
		_ = killProcessGroup(h.cmd.Process.Pid)
	}
	_ = h.cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = h.cmd.Wait()
		close(done)
	}()
	var err error
	select {
	case <-done:
	case <-time.After(catalogHelperReapTimeout):
		err = errors.New("catalog helper was not reaped")
	}
	releaseProcessLifecycle(h.lifecycle)
	_ = h.stdout.Close()
	return err
}
