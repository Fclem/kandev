package github

import (
	"context"
	"errors"
	"testing"
)

type personalStarClientStub struct {
	starred bool
	calls   int
}

func (c *personalStarClientStub) IsStarred(context.Context, GitHubRepositoryRef) (bool, error) {
	return c.starred, nil
}

func (c *personalStarClientStub) Star(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	c.starred = true
	return true, nil
}

func (c *personalStarClientStub) Unstar(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	c.starred = false
	return false, nil
}

type personalStarResolverStub struct {
	status PersonalStarConnectionStatus
	client PersonalStarClient
	calls  int
}

func (r *personalStarResolverStub) ConnectionStatus(context.Context, string, string) (PersonalStarConnectionStatus, error) {
	return r.status, nil
}

func (r *personalStarResolverStub) ResolvePersonalStarClient(context.Context, string, string) (PersonalStarClient, error) {
	r.calls++
	return r.client, nil
}

func TestServiceStarRepositoryAuthorizesBeforeCredentialResolution(t *testing.T) {
	client := &personalStarClientStub{}
	resolver := &personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetPersonalStarResolver(resolver)
	denied := errors.New("workspace denied")
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })

	_, err := service.StarRepository(context.Background(), "foreign-workspace", "user-1", "https://github.com/owner/repository")
	if !errors.Is(err, denied) {
		t.Fatalf("StarRepository() error = %v, want %v", err, denied)
	}
	if resolver.calls != 0 || client.calls != 0 {
		t.Fatalf("resolver calls = %d, client calls = %d; want no credential/provider call", resolver.calls, client.calls)
	}
}

func TestServiceRepositoryStarStatusDeduplicatesCanonicalURLs(t *testing.T) {
	client := &personalStarClientStub{starred: true}
	resolver := &personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetPersonalStarResolver(resolver)

	states, err := service.RepositoryStarStatus(context.Background(), "workspace-1", "user-1", []string{
		"https://github.com/Owner/Repository", "https://github.com/owner/repository.git",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := states["owner/repository"]; got == nil || !*got {
		t.Fatalf("states = %#v", states)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
}
