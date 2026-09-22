package deploymentsecret

import "testing"

func TestAvailabilityRejectsUnsupportedTargets(t *testing.T) {
	for _, target := range []Target{
		{GOOS: "darwin", BackendUID: 1000},
		{GOOS: "windows", BackendUID: 1000},
		{GOOS: linuxOS, BackendUID: 0},
		{GOOS: linuxOS, BackendUID: 1000, Container: true},
		{GOOS: linuxOS, BackendUID: 1000, UserSystemd: true},
	} {
		if Available(target) {
			t.Errorf("Available(%+v) = true", target)
		}
	}
}

func TestAvailabilityAcceptsLinuxSystemService(t *testing.T) {
	if !Available(Target{GOOS: linuxOS, BackendUID: 1000}) {
		t.Fatal("Available() = false")
	}
}

func TestValidateTargetExplainsUnavailableDeploymentSecretTargets(t *testing.T) {
	for _, target := range []Target{
		{GOOS: "darwin", BackendUID: 1000},
		{GOOS: linuxOS, BackendUID: 0},
		{GOOS: linuxOS, BackendUID: 1000, Container: true},
		{GOOS: linuxOS, BackendUID: 1000, UserSystemd: true},
	} {
		if err := ValidateTarget(target); err == nil {
			t.Fatalf("ValidateTarget(%+v) succeeded, want unavailable target error", target)
		}
	}
}
