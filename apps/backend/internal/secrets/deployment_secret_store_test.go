package secrets

import "testing"

func TestDeploymentSecretReferenceRequiresProtectedNamespace(t *testing.T) {
	if err := ValidateDeploymentSecretReference("kandev-runtime:github-personal-star:client-secret:1"); err != nil {
		t.Fatalf("ValidateDeploymentSecretReference() error = %v", err)
	}
	if err := ValidateDeploymentSecretReference("github:user:workspace:access"); err == nil {
		t.Fatal("ValidateDeploymentSecretReference() accepted user secret reference")
	}
}

func TestDeploymentSecretReferenceRejectsNonCanonicalVersions(t *testing.T) {
	for _, reference := range []string{
		" kandev-runtime:github-personal-star:client-secret:1",
		"kandev-runtime:github-personal-star:client-secret:01",
		"kandev-runtime:github-personal-star:client-secret:0",
		"kandev-runtime:github-personal-star:client-secret:-1",
	} {
		if err := ValidateDeploymentSecretReference(reference); err == nil {
			t.Fatalf("ValidateDeploymentSecretReference(%q) succeeded, want rejection", reference)
		}
	}
}
