//go:build !linux

package main

import (
	"io"
	"runtime"
)

// serve refuses to run off Linux: the socket peer check, the root-owned socket
// directory, and the systemd unit are Linux-only, and a Darwin/Windows
// deployment cannot provision secrets safely.
func serve(stderr io.Writer, config daemonConfig) int {
	logf(stderr, "error: the deployment secret supervisor is unavailable on %s (socket %s, data dir %s)",
		runtime.GOOS, config.socketPath, config.dataDir)
	return 1
}
