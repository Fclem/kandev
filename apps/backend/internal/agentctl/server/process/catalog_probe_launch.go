package process

import (
	"os/exec"
	"slices"
)

// catalogLaunch is the sealed final command of one successfully started
// process generation, captured after every environment, stripping, runtime
// and wrapper stage. It stays private to agentctl: it is never serialized,
// logged, returned or rebuilt from configuration, and empty values remain
// distinct from absent keys.
type catalogLaunch struct {
	generation uint64
	path       string
	args       []string
	env        []string
	dir        string
	agentType  string
	protocol   string
	wrapped    bool
	logical    []string
}

func sealCatalogLaunch(cmd *exec.Cmd, generation uint64, agentType, protocol string, wrapped bool, logical []string) *catalogLaunch {
	if cmd == nil || generation == 0 {
		return nil
	}
	return &catalogLaunch{
		generation: generation,
		path:       cmd.Path,
		args:       slices.Clone(cmd.Args),
		env:        cmd.Environ(),
		dir:        cmd.Dir,
		agentType:  agentType,
		protocol:   protocol,
		wrapped:    wrapped,
		logical:    slices.Clone(logical),
	}
}

// helperCommand recreates the sealed command for a fresh observation process.
// The executable is not re-resolved through PATH.
func (l *catalogLaunch) helperCommand() *exec.Cmd {
	return &exec.Cmd{Path: l.path, Args: slices.Clone(l.args), Env: slices.Clone(l.env), Dir: l.dir}
}
