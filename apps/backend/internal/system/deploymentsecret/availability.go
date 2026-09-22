package deploymentsecret

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
