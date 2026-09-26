package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

type personalStarClientStub struct {
	starred             bool
	calls               int
	statusCalls         int
	cancelStatus        context.CancelFunc
	statusDeadlines     []time.Time
	statusDeadlineIsSet []bool
}

func (c *personalStarClientStub) IsStarred(ctx context.Context, _ GitHubRepositoryRef) (bool, error) {
	c.statusCalls++
	deadline, ok := ctx.Deadline()
	c.statusDeadlines = append(c.statusDeadlines, deadline)
	c.statusDeadlineIsSet = append(c.statusDeadlineIsSet, ok)
	if c.cancelStatus != nil && c.statusCalls == 1 {
		c.cancelStatus()
		return false, ctx.Err()
	}
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

func TestServiceRepositoryStarStatusSharesOneBoundedDeadlineAcrossProviderCalls(t *testing.T) {
	client := &personalStarClientStub{}
	resolver := &personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetPersonalStarResolver(resolver)

	started := time.Now()
	_, err := service.RepositoryStarStatus(context.Background(), "workspace-1", "user-1", []string{
		"https://github.com/owner/first", "https://github.com/owner/second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.statusDeadlines) != 2 || len(client.statusDeadlineIsSet) != 2 {
		t.Fatalf("provider calls = %d, deadline flags = %d; want two calls with deadlines",
			len(client.statusDeadlines), len(client.statusDeadlineIsSet))
	}
	if !client.statusDeadlineIsSet[0] || !client.statusDeadlineIsSet[1] {
		t.Fatal("provider calls did not receive a bounded batch deadline")
	}
	if client.statusDeadlines[0] != client.statusDeadlines[1] {
		t.Fatalf("provider deadlines differ: %v and %v", client.statusDeadlines[0], client.statusDeadlines[1])
	}
	maxBudget := personalStarStatusBatchTimeout + time.Second
	if remaining := client.statusDeadlines[0].Sub(started); remaining <= 0 || remaining > maxBudget {
		t.Fatalf("batch deadline budget = %s, want (0, %s] including setup", remaining, maxBudget)
	}
}

func TestServiceRepositoryStarStatusStopsAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &personalStarClientStub{cancelStatus: cancel}
	resolver := &personalStarResolverStub{
		status: PersonalStarConnectionStatus{Available: true, Active: true, Capable: true},
		client: client,
	}
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	service.SetPersonalStarResolver(resolver)

	states, err := service.RepositoryStarStatus(ctx, "workspace-1", "user-1", []string{
		"https://github.com/owner/first", "https://github.com/owner/second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.statusCalls != 1 || resolver.calls != 1 {
		t.Fatalf("provider calls = %d, resolver calls = %d; want one of each", client.statusCalls, resolver.calls)
	}
	if len(states) != 2 || states["owner/first"] != nil || states["owner/second"] != nil {
		t.Fatalf("states = %#v, want both requested repositories unknown", states)
	}
}
