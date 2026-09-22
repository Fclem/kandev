package secrets

import (
	"context"
	"errors"
	"testing"
)

type deploymentSecretReaderStub struct {
	value string
	err   error
	seen  string
}

func (s *deploymentSecretReaderStub) RevealDeploymentSecret(_ context.Context, reference string) (string, error) {
	s.seen = reference
	return s.value, s.err
}

func TestDeploymentSecretResolverRejectsUnprotectedReferencesBeforeLookup(t *testing.T) {
	reader := &deploymentSecretReaderStub{value: "secret"}
	resolver := NewDeploymentSecretResolver(reader)

	if _, err := resolver.Resolve(context.Background(), "user-managed:secret"); err == nil {
		t.Fatal("Resolve() succeeded for unprotected reference")
	}
	if reader.seen != "" {
		t.Fatalf("reader received %q, want no lookup", reader.seen)
	}
}

func TestDeploymentSecretResolverReturnsProtectedValue(t *testing.T) {
	reader := &deploymentSecretReaderStub{value: "secret"}
	resolver := NewDeploymentSecretResolver(reader)
	reference := "kandev-runtime:github-personal-star:client-secret:1"

	value, err := resolver.Resolve(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if value != "secret" || reader.seen != reference {
		t.Fatalf("Resolve() = %q, lookup %q", value, reader.seen)
	}
}

func TestDeploymentSecretResolverFailsClosed(t *testing.T) {
	resolver := NewDeploymentSecretResolver(&deploymentSecretReaderStub{err: errors.New("unavailable")})
	if _, err := resolver.Resolve(context.Background(), "kandev-runtime:github-personal-star:client-secret:1"); err == nil {
		t.Fatal("Resolve() succeeded after reader failure")
	}
}
