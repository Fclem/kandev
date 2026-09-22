//go:build linux

package deploymentsecret

import (
	"fmt"
	"net"
	"syscall"
)

// UnixPeerUID obtains the kernel-authenticated UID for a connected Unix socket.
func UnixPeerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("access unix peer socket: %w", err)
	}
	var credential *syscall.Ucred
	var controlErr error
	err = raw.Control(func(fd uintptr) {
		credential, controlErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return 0, fmt.Errorf("inspect unix peer credentials: %w", err)
	}
	if controlErr != nil {
		return 0, fmt.Errorf("read unix peer credentials: %w", controlErr)
	}
	if credential == nil {
		return 0, fmt.Errorf("read unix peer credentials: missing credentials")
	}
	return credential.Uid, nil
}
