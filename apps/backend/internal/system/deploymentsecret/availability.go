package deploymentsecret

import "fmt"

const linuxOS = "linux"

// Target describes the backend execution context relevant to the root-owned
// deployment-secret supervisor boundary.
type Target struct {
	GOOS        string
	BackendUID  int
	Container   bool
	UserSystemd bool
}

// Available reports whether the platform can safely use the separate
// root-owned deployment-secret supervisor. Root-run, container, and
// user-systemd backends are rejected so the backend cannot become a provisioner.
func Available(target Target) bool {
	return target.GOOS == linuxOS && target.BackendUID != 0 && !target.Container && !target.UserSystemd
}

// ValidateTarget returns an explicit startup error for unsupported deployment
// secret targets. Callers must use it before socket creation or provisioning.
func ValidateTarget(target Target) error {
	switch {
	case target.GOOS != linuxOS:
		return fmt.Errorf("deployment secret daemon is unavailable on %s", target.GOOS)
	case target.BackendUID == 0:
		return fmt.Errorf("deployment secret daemon is unavailable to a root backend")
	case target.Container:
		return fmt.Errorf("deployment secret daemon is unavailable in a container")
	case target.UserSystemd:
		return fmt.Errorf("deployment secret daemon is unavailable to user systemd")
	default:
		return nil
	}
}
