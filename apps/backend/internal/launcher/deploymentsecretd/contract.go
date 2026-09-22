// Package deploymentsecretd holds the install contract shared by the
// root-owned deployment secret supervisor, the root-only operator CLI, and the
// released systemd unit: the socket path, the unit metadata, and the canonical
// install location.
//
// It deliberately has no dependencies. The supervisor binary embeds secrets
// access and nothing else; importing the launcher or the operator CLI into a
// process that runs as root would widen its surface for constants that must
// simply never disagree between the daemon, the CLI, and the shipped unit.
package deploymentsecretd

const (
	// SocketPath is the default root-owned supervisor socket. The unit's
	// ExecStart passes it explicitly and the operator CLI dials it by default.
	SocketPath = "/run/kandev/deployment-secret.sock"

	// BinaryName is the daemon binary name shipped in the release tarball.
	BinaryName = "kandev-deployment-secretd"

	// InstallPath is the canonical install path the released unit's ExecStart
	// points at.
	InstallPath = "/usr/local/bin/" + BinaryName

	// UnitName is the only systemd unit the daemon archive manages. The
	// installer never touches kandev.service.
	UnitName = BinaryName + ".service"

	// UnitPath is where systemd reads that unit from.
	UnitPath = "/etc/systemd/system/" + UnitName
)
