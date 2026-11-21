package process

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"go.uber.org/zap"
)

// ErrCatalogProbeBusy reports that this instance already owns a helper.
var ErrCatalogProbeBusy = sessioncatalog.ErrTaskProbeBusy

// catalogProbeCapability is one versioned fresh-acquisition implementation.
type catalogProbeCapability struct {
	id      string
	acquire func(context.Context, *catalogHelper) (sessioncatalog.Discovery, error)
}

type catalogProbeTarget struct {
	adapter adapter.AgentAdapter
	launch  *catalogLaunch
	owner   sessioncatalog.TaskProbeOwner
}

// SessionCatalogSource reports an adapter's handshake source epoch. Zero means
// the adapter cannot prove which New/Load/Resume handshake it serves.
func SessionCatalogSource(agentAdapter adapter.AgentAdapter) uint64 {
	provider, ok := agentAdapter.(interface{ SessionCatalogSourceGeneration() uint64 })
	if !ok || agentAdapter == nil {
		return 0
	}
	return provider.SessionCatalogSourceGeneration()
}

// DiscoverCatalogCandidates reads a fresh catalog in the sealed context of the
// requested live owner. It never touches the live adapter, conversation or
// process; ownership is checked before and after the bounded helper runs.
func (m *Manager) DiscoverCatalogCandidates(ctx context.Context, instanceID string, request sessioncatalog.TaskProbeRequest) (sessioncatalog.TaskProbeReply, error) {
	if err := request.Validate(); err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	target, err := m.captureCatalogProbeTarget(request, nil)
	if err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	if !m.catalogProbeActive.CompareAndSwap(false, true) {
		return sessioncatalog.TaskProbeReply{}, ErrCatalogProbeBusy
	}
	defer m.catalogProbeActive.Store(false)
	capability, qualified := m.qualifyCatalogLaunch(target.launch)
	result := unsupportedCatalogDiscovery()
	if qualified {
		if result, err = m.runCatalogHelper(ctx, target.launch, capability); err != nil {
			return sessioncatalog.TaskProbeReply{}, err
		}
	}
	if _, err := m.captureCatalogProbeTarget(request, target); err != nil {
		return sessioncatalog.TaskProbeReply{}, err
	}
	reply := sessioncatalog.TaskProbeReply{RequestID: request.RequestID, Owner: target.owner, Result: result}
	reply.Owner.InstanceID = instanceID
	if qualified {
		reply.Capability = capability.id
	}
	return reply, nil
}

// captureCatalogProbeTarget proves the requested owner under the startup lock.
// With a previous capture it also requires the identical adapter and launch,
// so same-ID replacement and same-process reload are both rejected.
func (m *Manager) captureCatalogProbeTarget(request sessioncatalog.TaskProbeRequest, previous *catalogProbeTarget) (*catalogProbeTarget, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if request.ManagerID != m.processManagerID || request.ProcessGeneration != m.ProcessGeneration() || m.Status() != StatusRunning {
		return nil, sessioncatalog.ErrTaskProbeOwner
	}
	current := m.GetAdapter()
	launch := m.catalogLaunch.Load()
	if current == nil || launch == nil || launch.generation != request.ProcessGeneration || current.GetSessionID() != request.SessionID {
		return nil, sessioncatalog.ErrTaskProbeOwner
	}
	source := SessionCatalogSource(current)
	if source == 0 || source != request.SessionSource {
		return nil, sessioncatalog.ErrTaskProbeOwner
	}
	if previous != nil && (previous.adapter != current || previous.launch != launch) {
		return nil, sessioncatalog.ErrTaskProbeOwner
	}
	return &catalogProbeTarget{adapter: current, launch: launch, owner: sessioncatalog.TaskProbeOwner{
		ManagerID: m.processManagerID, ProcessGeneration: request.ProcessGeneration,
		SessionID: request.SessionID, SessionSource: source,
	}}, nil
}

func (m *Manager) qualifyCatalogLaunch(launch *catalogLaunch) (catalogProbeCapability, bool) {
	if m.catalogProbeQualifier != nil {
		return m.catalogProbeQualifier(launch)
	}
	return qualifyCatalogLaunch(launch)
}

// runCatalogHelper bounds one fresh helper. Helper failures become a sanitized
// failed observation; only request cancellation is returned as an error.
func (m *Manager) runCatalogHelper(ctx context.Context, launch *catalogLaunch, capability catalogProbeCapability) (sessioncatalog.Discovery, error) {
	helperCtx, cancel := context.WithTimeout(ctx, sessioncatalog.TaskProbeHelperTimeout)
	defer cancel()
	helper, err := startCatalogHelper(launch)
	if err != nil {
		m.logCatalogHelper("helper_unavailable", capability.id, 0)
		return failedCatalogDiscovery(), nil
	}
	result, acquireErr := capability.acquire(helperCtx, helper)
	stopErr := helper.stop()
	if err := ctx.Err(); err != nil {
		return sessioncatalog.Discovery{}, err
	}
	switch {
	case acquireErr != nil:
		m.logCatalogHelper("acquisition_failed", capability.id, helper.stderr.size())
		return failedCatalogDiscovery(), nil
	case stopErr != nil:
		m.logCatalogHelper("helper_not_reaped", capability.id, helper.stderr.size())
		return failedCatalogDiscovery(), nil
	case sessioncatalog.ValidateDiscovery(&result) != nil:
		m.logCatalogHelper("invalid_result", capability.id, helper.stderr.size())
		return failedCatalogDiscovery(), nil
	}
	return result, nil
}

func (m *Manager) logCatalogHelper(reason, capability string, stderrBytes int) {
	if m.logger == nil {
		return
	}
	m.logger.Info("task catalog helper did not complete",
		zap.String("reason", reason), zap.String("capability", capability), zap.Int("stderr_bytes", stderrBytes))
}
