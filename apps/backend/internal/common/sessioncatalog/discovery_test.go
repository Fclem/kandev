package sessioncatalog

import "testing"

func TestDiscoveryRejectsCompleteWithoutObservedSupportedSurface(t *testing.T) {
	result := &Discovery{
		Outcome: "complete",
		Models:  Surface{Supported: true},
		Modes:   Surface{Present: true},
	}
	if err := ValidateDiscovery(result); err == nil {
		t.Fatal("complete discovery accepted support and presence from different surfaces")
	}
}
