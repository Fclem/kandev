//go:build !linux

package deploymentsecret

import (
	"fmt"
	"net"
)

// UnixPeerUID is unsupported off Linux because the deployment daemon itself is
// unavailable there.
func UnixPeerUID(_ *net.UnixConn) (uint32, error) {
	return 0, fmt.Errorf("unix peer credentials are unavailable on this platform")
}
