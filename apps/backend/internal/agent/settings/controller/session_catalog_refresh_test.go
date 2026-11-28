package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type catalogPublisherInstaller interface {
	SetCatalogRefreshPublisher(CatalogRefreshPublisher)
}

// @covers AC-AGENTS-SESSION-CATALOG-001.3
func TestSavedRefreshDoesNotPublishDraftOrFailedProbe(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	profile := &models.AgentProfile{AgentID: agentID, Name: "Saved", Model: "saved-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	utility := &recordingProfileDiscoveryUtility{}
	ctrl.hostUtility = utility
	ctrl.SetHostCLIRunner(&stubCLIRunner{listJSON: codexListJSON})
	installer, ok := any(ctrl).(catalogPublisherInstaller)
	if !ok {
		t.Fatal("successful saved refresh has no publication boundary")
	}
	var published []string
	installer.SetCatalogRefreshPublisher(func(ctx context.Context, id, agent string, commit func(context.Context, func() error) error) error {
		if id != profile.ID || agent != "codex-acp" {
			t.Fatal("publication lost saved profile identity")
		}
		return commit(ctx, func() error { published = append(published, id); return nil })
	})
	if _, err := ctrl.FetchProfileDynamicModels(ctx, "codex-acp", dto.ProfileCapabilityRequest{ProfileID: profile.ID, AuthorizationScope: "user-1", Refresh: true}); err != nil {
		t.Fatal(err)
	}
	if len(published) != 0 {
		t.Fatal("pure discovery published a session update")
	}
	result, err := ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Models) < 2 || result.Models[0].ID == "model-a" {
		t.Fatalf("test did not enable separate host-CLI enrichment: %+v", result.Models)
	}
	if len(published) != 1 {
		t.Fatalf("saved publication = %+v, want exactly one gated fan-out", published)
	}
	assertCatalogPublicationStatus(t, result, "published")
	utility.capabilityResult = &hostutility.ProfileCapabilityResult{Capabilities: hostutility.AgentCapabilities{Status: hostutility.StatusFailed, Error: "TOKEN=secret"}}
	result, err = ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 {
		t.Fatal("failed probe published a session update")
	}
	assertCatalogPublicationStatus(t, result, "not_published")
	installer.SetCatalogRefreshPublisher(func(context.Context, string, string, func(context.Context, func() error) error) error {
		return errors.New("TOKEN=private-session")
	})
	utility.capabilityResult = nil
	logs := observeControllerWarnings(t, ctrl)
	result, err = ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogPublicationStatus(t, result, "failed")
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "TOKEN=") {
		t.Fatal("publication failure leaked internal error")
	}
	failures := logs.FilterMessage("saved catalog publication failed").All()
	if len(failures) != 1 {
		t.Fatalf("publication failure logs = %d, want one aggregate entry", len(failures))
	}
	fields := failures[0].ContextMap()
	if fields["reason"] != "fanout_failed" || fields["profile_id"] != profile.ID || fields["agent"] != "codex-acp" {
		t.Fatalf("publication failure log fields = %+v", fields)
	}
	assertNoLoggedSecret(t, logs, "TOKEN=")
	saved, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Model != "saved-model" {
		t.Fatal("refresh changed saved selection")
	}
}

func assertCatalogPublicationStatus(t *testing.T, response any, want string) {
	t.Helper()
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Publication struct {
			Status string `json:"status"`
		} `json:"publication"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Publication.Status != want {
		t.Fatalf("publication status = %q, want %q", wire.Publication.Status, want)
	}
}

type changingCatalogUtility struct {
	recordingProfileDiscoveryUtility
	afterProbe func()
}

func (u *changingCatalogUtility) ProbeProfileCapabilities(ctx context.Context, agent string, req hostutility.ProfileCapabilityRequest) (hostutility.ProfileCapabilityResult, error) {
	result, err := u.recordingProfileDiscoveryUtility.ProbeProfileCapabilities(ctx, agent, req)
	if after := u.afterProbe; after != nil {
		u.afterProbe = nil
		after()
	}
	return result, err
}

// @covers AC-AGENTS-SESSION-CATALOG-001.3
// @covers AC-AGENTS-SESSION-CATALOG-001.6
func TestSavedProfileCatalogRefreshRejectsSupersededProbe(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	profile := &models.AgentProfile{AgentID: agentID, Name: "Saved", Model: "saved-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	utility := &changingCatalogUtility{}
	ctrl.hostUtility = utility
	published := 0
	ctrl.SetCatalogRefreshPublisher(func(ctx context.Context, _, _ string, commit func(context.Context, func() error) error) error {
		return commit(ctx, func() error { published++; return nil })
	})
	utility.afterProbe = func() {
		fresh, err := ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		assertCatalogPublicationStatus(t, fresh, "published")
	}
	old, err := ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatalf("superseded probe published: %d writes, want only the newer refresh", published)
	}
	assertCatalogPublicationStatus(t, old, "failed")
}

// @covers AC-AGENTS-SESSION-CATALOG-001.3
func TestSavedProfileCatalogRefreshRechecksContextDuringFanout(t *testing.T) {
	for _, mutation := range []string{"profile", "cancel"} {
		t.Run(mutation, func(t *testing.T) {
			ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
			profile := &models.AgentProfile{AgentID: agentID, Name: "Saved", Model: "saved-model"}
			if err := repo.CreateAgentProfile(ctx, profile); err != nil {
				t.Fatal(err)
			}
			ctrl.hostUtility = &recordingProfileDiscoveryUtility{}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			committed := 0
			ctrl.SetCatalogRefreshPublisher(func(ctx context.Context, _, _ string, commit func(context.Context, func() error) error) error {
				if err := commit(ctx, func() error { committed++; return nil }); err != nil {
					return err
				}
				if mutation == "profile" {
					profile.CommandPrefix = "changed-wrapper --"
					if err := repo.UpdateAgentProfile(ctx, profile); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				return commit(ctx, func() error { committed++; return nil })
			})
			result, err := ctrl.RefreshProfileModelCatalog(ctx, profile.ID, "user-1")
			if err != nil {
				t.Fatal(err)
			}
			assertCatalogPublicationStatus(t, result, "failed")
			if committed != 1 {
				t.Fatalf("changed context wrote %d sessions, want only earlier committed session", committed)
			}
		})
	}
}

func (u *recordingProfileDiscoveryUtility) CommitProfileCapability(ctx context.Context, _ hostutility.ProfileCapabilityResult, commit func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return commit()
}

func observeControllerWarnings(t *testing.T, ctrl *Controller) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	ctrl.logger = log
	return logs
}

func assertNoLoggedSecret(t *testing.T, logs *observer.ObservedLogs, secret string) {
	t.Helper()
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(entry.Message, secret) || strings.Contains(string(encoded), secret) {
			t.Fatalf("log %q leaked %q: %s", entry.Message, secret, encoded)
		}
	}
}
