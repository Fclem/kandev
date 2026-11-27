package process

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

const (
	codexAppServerAgentType = "codex-app-server"
	mockCatalogAgentType    = "mock-agent"
)

// qualifyCatalogLaunch is the versioned capability table. Only an unwrapped
// built-in launch with a positively identified shape qualifies; ACP, custom
// commands, wrappers, CLI flags and other versions remain unsupported. The
// mock fixture capability is test support, never provider qualification.
func qualifyCatalogLaunch(launch *catalogLaunch) (catalogProbeCapability, bool) {
	if launch == nil || launch.wrapped {
		return catalogProbeCapability{}, false
	}
	switch {
	case launch.protocol == string(agent.ProtocolCodexAppServer) && launch.agentType == codexAppServerAgentType:
		_, invocation, ok := agents.MatchBuiltinManagedNPMRuntime(launch.agentType, launch.logical)
		if !ok || len(invocation.AgentArgs()) != 0 {
			break
		}
		if qualification, qualified := codexQualificationFor(invocation.PackageSpec); qualified {
			return codexModelListCapability(qualification), true
		}
	case launch.protocol == string(agent.ProtocolACP) && launch.agentType == mockCatalogAgentType:
		return mockCatalogCapability(), true
	}
	return catalogProbeCapability{}, false
}
