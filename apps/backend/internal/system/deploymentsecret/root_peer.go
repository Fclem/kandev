package deploymentsecret

import "fmt"

// AuthorizeRootPeer enforces the daemon's privileged caller boundary.
func AuthorizeRootPeer(uid uint32) error {
	if uid != 0 {
		return fmt.Errorf("deployment secret daemon requires a root peer")
	}
	return nil
}
