package deploymentsecret

import (
	"os"
	"runtime"
	"strings"
)

// Target environment markers. The launcher writes KANDEV_SERVICE_MODE into
// every service unit it installs ("system" or "user"), and container runtimes
// leave at least one of these markers behind.
const (
	serviceModeEnvVar = "KANDEV_SERVICE_MODE"
	serviceModeUser   = "user"
)

// DetectBackendTarget reports the runtime facts that decide whether this
// backend may use the root-owned deployment-secret supervisor. A backend
// running as UID 0 is unsupported by design: it would satisfy the same socket
// peer check as the operator CLI, so it could provision its own credentials.
func DetectBackendTarget() Target {
	return Target{
		GOOS:        runtime.GOOS,
		BackendUID:  os.Geteuid(),
		Container:   runningInContainer(),
		UserSystemd: strings.TrimSpace(os.Getenv(serviceModeEnvVar)) == serviceModeUser,
	}
}

func runningInContainer() bool {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	content, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := string(content)
	for _, marker := range []string{"docker", "kubepods", "containerd", "podman", "lxc"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
