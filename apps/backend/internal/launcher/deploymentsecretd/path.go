package deploymentsecretd

import (
	"errors"
	"fmt"
)

// ErrSocketPathTooLong reports a resolved socket path the kernel cannot bind.
var ErrSocketPathTooLong = errors.New("deployment secret socket path is too long")

// UnixSocketPathLimit is the longest unix socket path this platform can bind.
// Linux and macOS store the path in sun_path, which is 108 bytes including the
// NUL terminator, so the path itself may be at most 107 bytes. A longer path
// fails bind with EINVAL and no usable diagnostic.
const UnixSocketPathLimit = 107

// ValidateSocketPath fails closed before any socket side effect when a resolved
// path cannot be bound. The daemon, the operator CLI, and the unit generator all
// resolve this path, so the rule lives with the contract they share rather than
// being restated at each call site.
func ValidateSocketPath(path string) error {
	if len(path) <= UnixSocketPathLimit {
		return nil
	}
	return fmt.Errorf("%w: %s is %d bytes, above the %d-byte unix socket limit: pass a shorter --socket value (the default %s always fits)",
		ErrSocketPathTooLong, path, len(path), UnixSocketPathLimit, SocketPath)
}
