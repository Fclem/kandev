package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const (
	runtimeUpdateStatusSuccessTTL    = 6 * time.Hour
	runtimeUpdateStatusFailureTTL    = 15 * time.Minute
	runtimeUpdateStatusMaxConcurrent = 5
	runtimeUpdateStatusLookupTimeout = 10 * time.Second
)

// RuntimeUpdateStatusResolver is the latest-version lookup seam used by the
// cached status endpoint. Production resolves through the configured runtime
// updater; tests and embedders can provide a deterministic implementation.
type RuntimeUpdateStatusResolver func(context.Context, string) (string, error)

type runtimeUpdateStatusCacheEntry struct {
	latest    string
	checkedAt time.Time
	expiresAt time.Time
	ok        bool
}

type runtimeUpdateStatusTarget struct {
	agentName        string
	updateMode       dto.AgentUpdateMode
	packageName      string
	defaultVersion   string
	activeVersion    string
	effectiveVersion string
	selectionErr     error
}

// SetRuntimeUpdateStatusClock injects the clock used for status cache TTLs.
// Passing nil restores the wall clock.
func (c *Controller) SetRuntimeUpdateStatusClock(now func() time.Time) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if now == nil {
		c.runtimeUpdateStatusNow = time.Now
		return
	}
	c.runtimeUpdateStatusNow = now
}

// SetRuntimeUpdateStatusResolver injects the read-only latest-version lookup
// used by the status endpoint and clears prior results from another resolver.
func (c *Controller) SetRuntimeUpdateStatusResolver(resolver RuntimeUpdateStatusResolver) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	c.runtimeUpdateStatusResolver = resolver
	c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
}

// InvalidateRuntimeUpdateStatus expires only the affected package. Successful
// activation and return-to-default jobs call this after persistence/probing.
func (c *Controller) InvalidateRuntimeUpdateStatus(packageName string) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return
	}
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	delete(c.runtimeUpdateStatusCache, packageName)
}

// ListAgentUpdateStatuses returns one non-mutating status item for each
// available built-in managed runtime. Registry failures are represented as
// unknown entries rather than failing the whole batch.
func (c *Controller) ListAgentUpdateStatuses(ctx context.Context) (*dto.ListAgentUpdateStatusResponse, error) {
	targets, err := c.runtimeUpdateStatusTargets(ctx)
	if err != nil {
		return nil, err
	}

	now := c.runtimeUpdateStatusTime()
	entries := make(map[string]runtimeUpdateStatusCacheEntry, len(targets))
	uniquePackages := make(map[string]dto.AgentUpdateMode, len(targets))
	for _, target := range targets {
		uniquePackages[target.packageName] = target.updateMode
	}
	type result struct {
		packageName string
		entry       runtimeUpdateStatusCacheEntry
	}
	results := make(chan result, len(uniquePackages))
	for packageName, mode := range uniquePackages {
		go func(packageName string, mode dto.AgentUpdateMode) {
			entry := c.runtimeUpdateStatusEntry(ctx, packageName, mode, now)
			results <- result{packageName: packageName, entry: entry}
		}(packageName, mode)
	}
	for range uniquePackages {
		item := <-results
		entries[item.packageName] = item.entry
	}

	statuses := make([]dto.AgentUpdateStatusDTO, 0, len(targets))
	for _, target := range targets {
		entry := entries[target.packageName]
		status := dto.AgentUpdateStatusDTO{
			UpdateMode:       target.updateMode,
			AgentName:        target.agentName,
			Package:          target.packageName,
			DefaultVersion:   target.defaultVersion,
			ActiveVersion:    target.activeVersion,
			EffectiveVersion: target.effectiveVersion,
			CheckState:       dto.AgentUpdateCheckStateUnknown,
		}
		if !entry.checkedAt.IsZero() {
			checkedAt := entry.checkedAt
			status.CheckedAt = &checkedAt
		}
		if entry.ok {
			status.LatestVersion = entry.latest
		}
		if target.selectionErr == nil && entry.ok {
			if target.updateMode == dto.AgentUpdateModeSelfUpdate {
				switch harnessUpdateOperation(target.effectiveVersion, entry.latest) {
				case string(managedruntime.OperationUpToDate):
					status.CheckState = dto.AgentUpdateCheckStateUpToDate
				case string(managedruntime.OperationUpdate):
					status.CheckState = dto.AgentUpdateCheckStateUpdateAvailable
				}
			} else {
				status.CheckState = compareRuntimeUpdateStatus(entry.latest, target.effectiveVersion)
			}
		}
		statuses = append(statuses, status)
	}
	return &dto.ListAgentUpdateStatusResponse{Statuses: statuses}, nil
}

func (c *Controller) runtimeUpdateStatusTargets(ctx context.Context) ([]runtimeUpdateStatusTarget, error) {
	if c.agentRegistry == nil {
		return nil, nil
	}
	available := map[string]bool{}
	if c.discovery != nil {
		results, err := c.detectAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("detect available agents: %w", err)
		}
		for _, result := range results {
			available[result.Name] = result.Available
		}
	}

	targets := make([]runtimeUpdateStatusTarget, 0)
	for _, ag := range c.agentRegistry.ListEnabled() {
		if c.discovery != nil && !available[ag.ID()] {
			continue
		}
		managed, pinned := ag.(agents.ManagedNPMRuntimeAgent)
		if !pinned {
			harness, ok := ag.(agents.HarnessUpdateAgent)
			if !ok {
				continue
			}
			spec := harness.HarnessUpdate()
			if strings.TrimSpace(spec.Package) == "" || spec.UpdateCommand.IsEmpty() {
				continue
			}
			current := c.harnessCurrentVersion(ag.ID())
			targets = append(targets, runtimeUpdateStatusTarget{
				agentName: ag.ID(), packageName: spec.Package,
				updateMode: dto.AgentUpdateModeSelfUpdate, effectiveVersion: current,
			})
			continue
		}
		spec := managed.ManagedNPMRuntime()
		packageName := strings.TrimSpace(spec.Package)
		if packageName == "" {
			continue
		}
		defaultVersion := strings.TrimSpace(spec.DefaultVersionOrPinned())
		if _, err := managedruntime.ParseStableVersion(defaultVersion); err != nil {
			// Custom test/extension agents are not part of the built-in managed
			// catalogue and must not create an unverifiable status item.
			continue
		}
		activeVersion, effectiveVersion, _, selectionErr := c.runtimeVersions(ctx, ag.ID(), spec)
		if selectionErr != nil {
			effectiveVersion = defaultVersion
		}
		targets = append(targets, runtimeUpdateStatusTarget{
			updateMode:       dto.AgentUpdateModePinned,
			agentName:        ag.ID(),
			packageName:      packageName,
			defaultVersion:   defaultVersion,
			activeVersion:    activeVersion,
			effectiveVersion: effectiveVersion,
			selectionErr:     selectionErr,
		})
	}
	return targets, nil
}

func (c *Controller) runtimeUpdateStatusEntry(
	ctx context.Context,
	packageName string,
	mode dto.AgentUpdateMode,
	now time.Time,
) runtimeUpdateStatusCacheEntry {
	c.runtimeUpdateStatusMu.Lock()
	if entry, ok := c.runtimeUpdateStatusCache[packageName]; ok && now.Before(entry.expiresAt) {
		c.runtimeUpdateStatusMu.Unlock()
		return entry
	}
	lookup := c.runtimeUpdateStatusLookup
	if lookup == nil {
		lookup = make(chan struct{}, runtimeUpdateStatusMaxConcurrent)
		c.runtimeUpdateStatusLookup = lookup
	}
	c.runtimeUpdateStatusMu.Unlock()

	lookup <- struct{}{}
	defer func() { <-lookup }()

	// Recheck after waiting for the bounded slot so concurrent requests do not
	// repeat a lookup that another waiter already completed.
	c.runtimeUpdateStatusMu.Lock()
	if entry, ok := c.runtimeUpdateStatusCache[packageName]; ok && now.Before(entry.expiresAt) {
		c.runtimeUpdateStatusMu.Unlock()
		return entry
	}
	c.runtimeUpdateStatusMu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, runtimeUpdateStatusLookupTimeout)
	defer cancel()
	latest, err := c.resolveRuntimeUpdateLatest(lookupCtx, packageName, mode)
	entry := runtimeUpdateStatusCacheEntry{}
	if err == nil {
		entry.latest = latest
		entry.ok = true
		entry.checkedAt = now
		entry.expiresAt = now.Add(runtimeUpdateStatusSuccessTTL)
	} else {
		entry.expiresAt = now.Add(runtimeUpdateStatusFailureTTL)
	}
	c.runtimeUpdateStatusMu.Lock()
	if c.runtimeUpdateStatusCache == nil {
		c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
	}
	c.runtimeUpdateStatusCache[packageName] = entry
	c.runtimeUpdateStatusMu.Unlock()
	return entry
}

func (c *Controller) resolveRuntimeUpdateLatest(ctx context.Context, packageName string, mode dto.AgentUpdateMode) (string, error) {
	c.runtimeUpdateStatusMu.Lock()
	resolver := c.runtimeUpdateStatusResolver
	c.runtimeUpdateStatusMu.Unlock()
	if resolver != nil {
		return validateRuntimeUpdateLatest(resolver(ctx, packageName))
	}
	if mode == dto.AgentUpdateModeSelfUpdate {
		return c.resolveHarnessLatest(ctx, packageName)
	}
	if c.runtimeUpdater == nil {
		return "", errors.New("runtime updater unavailable")
	}
	if metadataResolver, ok := c.runtimeUpdater.(RuntimeVersionResolver); ok {
		metadata, err := metadataResolver.ResolveVersions(ctx, packageName)
		if err != nil {
			return "", err
		}
		return validateRuntimeUpdateLatest(metadata.Latest, nil)
	}
	return validateRuntimeUpdateLatest(c.runtimeUpdater.ResolveTarget(ctx, packageName))
}

func validateRuntimeUpdateLatest(latest string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	latest = strings.TrimSpace(latest)
	if _, parseErr := managedruntime.ParseStableVersion(latest); parseErr != nil {
		return "", parseErr
	}
	return latest, nil
}

func compareRuntimeUpdateStatus(latest, effective string) dto.AgentUpdateCheckState {
	latestVersion, latestErr := managedruntime.ParseStableVersion(latest)
	effectiveVersion, effectiveErr := managedruntime.ParseStableVersion(effective)
	if latestErr != nil || effectiveErr != nil {
		return dto.AgentUpdateCheckStateUnknown
	}
	if latestVersion.GreaterThan(effectiveVersion) {
		return dto.AgentUpdateCheckStateUpdateAvailable
	}
	return dto.AgentUpdateCheckStateUpToDate
}

func (c *Controller) runtimeUpdateStatusTime() time.Time {
	c.runtimeUpdateStatusMu.Lock()
	now := c.runtimeUpdateStatusNow
	c.runtimeUpdateStatusMu.Unlock()
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}
