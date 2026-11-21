package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryCapabilityTable(t *testing.T) {
	native := func(version string, extra ...string) []string {
		command := agents.NewCodexAppServer(true).BuildCommand(agents.CommandOptions{ManagedRuntimeVersion: version}).Args()
		return append(command, extra...)
	}
	codex := string(agent.ProtocolCodexAppServer)
	acp := string(agent.ProtocolACP)
	cases := []struct {
		name   string
		launch *catalogLaunch
		want   string
	}{
		{name: "qualified native app-server", launch: &catalogLaunch{protocol: codex, agentType: "codex-app-server", logical: native("0.169.0")}, want: codexCatalogCapabilityID},
		{name: "other native version", launch: &catalogLaunch{protocol: codex, agentType: "codex-app-server", logical: native("0.166.0")}},
		{name: "profile CLI flags", launch: &catalogLaunch{protocol: codex, agentType: "codex-app-server", logical: native("0.169.0", "--config", "x=1")}},
		{name: "wrapped launch", launch: &catalogLaunch{protocol: codex, agentType: "codex-app-server", logical: native("0.169.0"), wrapped: true}},
		{name: "custom command", launch: &catalogLaunch{protocol: codex, agentType: "codex-app-server", logical: []string{"codex", "app-server"}}},
		{name: "ACP provider", launch: &catalogLaunch{protocol: acp, agentType: "claude-acp", logical: []string{"npx", "claude"}}},
		{name: "unknown agent with native protocol", launch: &catalogLaunch{protocol: codex, agentType: "custom", logical: native("0.169.0")}},
		{name: "mock fixture", launch: &catalogLaunch{protocol: acp, agentType: "mock-agent", logical: []string{"mock-agent"}}, want: mockCatalogCapabilityID},
		{name: "wrapped mock fixture", launch: &catalogLaunch{protocol: acp, agentType: "mock-agent", wrapped: true}},
		{name: "missing launch evidence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capability, ok := qualifyCatalogLaunch(tc.launch)
			if ok != (tc.want != "") || capability.id != tc.want {
				t.Fatalf("capability = %q/%v, want %q", capability.id, ok, tc.want)
			}
		})
	}
}
