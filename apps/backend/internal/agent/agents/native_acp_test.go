package agents

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/pkg/agent"
)

// nativeACPCase pins the registered definition of one native-binary ACP agent
// — everything nativeACPSpec feeds into Agent. These agents share a single
// constructor (newNativeACPPassthrough / nativeACPAgent), so a slip in the
// shared scaffold would otherwise silently change eight agents at once:
// a stray shared default, a spec field wired to the wrong RuntimeConfig
// member, or an aliased argv slice.
type nativeACPCase struct {
	name string
	// newAgent builds the agent under test.
	newAgent func() Agent
	// Identity.
	id           string
	agentName    string
	displayName  string
	description  string
	displayOrder int
	// Launch surfaces. argv is asserted on BuildCommand, Runtime().Cmd and
	// InferenceConfig().Command; passthroughArgv on PassthroughCmd.
	bin  string
	argv []string
	// Passthrough. wantPassthrough=false means the agent must NOT satisfy
	// PassthroughAgent (Grok).
	wantPassthrough   bool
	modelFlag         []string
	resumeFlag        []string
	sessionResumeFlag []string
	mcpStrategy       mcpconfig.PassthroughMCPStrategy
	// Runtime.
	requiredEnv                 []string
	projectSkillDir             string
	userSkillDir                string
	projectMCPStrategy          mcpconfig.PassthroughMCPStrategy
	assumeMcpSse                bool
	assumeMcpHttp               bool
	stripEnv                    []string
	sessionDirTemplate          string
	sessionDirTarget            string
	newSessionOnWorkspaceRebind bool
	// Auth / install.
	installScript   string
	wantRemoteAuth  bool
	remoteAuthTypes []string
	permSettingKeys []string
}

var nativeACPCases = []nativeACPCase{
	{
		name: "cursor", newAgent: func() Agent { return NewCursorACP() },
		id: "cursor-acp", agentName: "Cursor ACP Agent", displayName: "Cursor",
		description:  "Cursor CLI coding agent (cursor-agent) using the ACP protocol. Requires a Cursor Pro subscription.",
		displayOrder: 13,
		bin:          "cursor-agent", argv: []string{"cursor-agent", "acp"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		mcpStrategy:        mcpconfig.CursorStrategy{},
		userSkillDir:       ".cursor/skills",
		sessionDirTemplate: "{home}/.cursor",
		projectMCPStrategy: mcpconfig.CursorStrategy{},
		installScript:      cursorACPInstallScript,
		wantRemoteAuth:     true, remoteAuthTypes: []string{"env"},
		permSettingKeys: []string{PermissionKeyCursorForce},
	},
	{
		name: "kimi", newAgent: func() Agent { return NewKimiACP() },
		id: "kimi-acp", agentName: "Kimi ACP Agent", displayName: "Kimi",
		description:  "Moonshot AI Kimi coding agent using the ACP protocol over stdin/stdout.",
		displayOrder: 14,
		bin:          "kimi", argv: []string{"kimi", "acp"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		sessionDirTemplate: "{home}/.kimi",
		installScript:      "Install Kimi CLI from Moonshot AI",
	},
	{
		name: "kiro", newAgent: func() Agent { return NewKiroACP() },
		id: "kiro-acp", agentName: "Kiro ACP Agent", displayName: "Kiro",
		description:  "AWS Kiro coding agent using the ACP protocol via kiro-cli-chat.",
		displayOrder: 15,
		bin:          "kiro-cli-chat", argv: []string{"kiro-cli-chat", "acp"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		userSkillDir:       ".kiro/skills",
		sessionDirTemplate: "{home}/.kiro",
		installScript:      "Install Kiro CLI from AWS",
	},
	{
		name: "qoder", newAgent: func() Agent { return NewQoderACP() },
		id: "qoder-acp", agentName: "Qoder ACP Agent", displayName: "Qoder",
		description:  "Qoder coding agent using the ACP protocol via qodercli --acp.",
		displayOrder: 16,
		bin:          "qodercli", argv: []string{"qodercli", "--acp"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		sessionDirTemplate: "{home}/.qoder",
		installScript:      "Install Qoder CLI from https://qoder.com",
	},
	{
		name: "trae", newAgent: func() Agent { return NewTraeACP() },
		id: "trae-acp", agentName: "Trae ACP Agent", displayName: "Trae",
		description:  "ByteDance Trae IDE coding agent using the ACP protocol via traecli acp serve.",
		displayOrder: 17,
		bin:          "traecli", argv: []string{"traecli", "acp", "serve"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		sessionDirTemplate: "{home}/.cache/trae-cli",
		installScript:      "Install Trae IDE CLI from https://trae.ai",
	},
	{
		name: "omp", newAgent: func() Agent { return NewOmpACP() },
		id: "omp-acp", agentName: "Oh My Pi ACP Agent", displayName: "omp",
		description:  "Oh My Pi (omp) coding agent using the ACP protocol via the `omp acp` subcommand.",
		displayOrder: 18,
		bin:          "omp", argv: []string{"omp", "acp"},
		wantPassthrough: true, modelFlag: []string{"--model", "{model}"},
		resumeFlag: []string{"-c"}, sessionResumeFlag: []string{"--resume"},
		projectSkillDir:    ".omp/skills",
		userSkillDir:       ".omp/agent/skills",
		sessionDirTemplate: "{home}/.omp",
		installScript:      "bun install -g @oh-my-pi/pi-coding-agent",
	},
	{
		name: "devin", newAgent: func() Agent { return NewDevinACP() },
		id: "devin-acp", agentName: "Devin ACP Agent", displayName: "Devin",
		description:  "Cognition Devin coding agent using the ACP protocol via `devin acp`.",
		displayOrder: 19,
		bin:          "devin", argv: []string{"devin", "acp"},
		// Devin's CLI takes no --model flag in passthrough mode.
		wantPassthrough: true,
		assumeMcpSse:    true, assumeMcpHttp: true,
		stripEnv:           []string{"ACP_BACKEND"},
		sessionDirTemplate: "{home}/.local/share/devin",
		sessionDirTarget:   "/root/.local/share/devin",
		installScript:      devinACPInstallScript,
		wantRemoteAuth:     true, remoteAuthTypes: []string{"files", "env"},
	},
	{
		name: "grok", newAgent: func() Agent { return NewGrokACP() },
		id: "grok-acp", agentName: "Grok ACP Agent", displayName: "Grok",
		description:  "xAI Grok coding agent using the ACP protocol over stdin/stdout.",
		displayOrder: 20,
		bin:          "grok", argv: []string{"grok", "--no-auto-update", "agent", "stdio"},
		// Raw TUI passthrough is deferred for Grok (ADR 0014).
		wantPassthrough:             false,
		requiredEnv:                 []string{},
		projectSkillDir:             ".grok/skills",
		userSkillDir:                ".grok/skills",
		sessionDirTemplate:          "{home}/.grok",
		sessionDirTarget:            "/root/.grok",
		newSessionOnWorkspaceRebind: true,
		installScript:               "npm install -g @xai-official/grok",
		wantRemoteAuth:              true, remoteAuthTypes: []string{"files", "env"},
	},
}

func TestNativeACPAgents_Identity(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			ag := tc.newAgent()
			assertString(t, "ID", ag.ID(), tc.id)
			assertString(t, "Name", ag.Name(), tc.agentName)
			assertString(t, "DisplayName", ag.DisplayName(), tc.displayName)
			assertString(t, "Description", ag.Description(), tc.description)
			if got := ag.DisplayOrder(); got != tc.displayOrder {
				t.Errorf("DisplayOrder() = %d, want %d", got, tc.displayOrder)
			}
			if !ag.Enabled() {
				t.Error("Enabled() = false, want true")
			}
			if len(ag.Logo(LogoLight)) == 0 || len(ag.Logo(LogoDark)) == 0 {
				t.Errorf("Logo lengths = (%d, %d), want both non-empty",
					len(ag.Logo(LogoLight)), len(ag.Logo(LogoDark)))
			}
		})
	}
}

// TestNativeACPAgents_LaunchArgv pins the three launch surfaces that must
// agree: BuildCommand, Runtime().Cmd, and InferenceConfig().Command. The
// shared spec derives all three from one `bin` + `args` pair, so this also
// catches a spec whose bin and args disagree with its documented CLI.
func TestNativeACPAgents_LaunchArgv(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			ag := tc.newAgent()
			assertArgv(t, "BuildCommand", ag.BuildCommand(CommandOptions{}).Args(), tc.argv)
			assertArgv(t, "Runtime.Cmd", ag.Runtime().Cmd.Args(), tc.argv)

			ia, ok := ag.(InferenceAgent)
			if !ok {
				t.Fatalf("%s does not implement InferenceAgent", tc.id)
			}
			ic := ia.InferenceConfig()
			if ic == nil || !ic.Supported {
				t.Fatalf("InferenceConfig() = %+v, want Supported=true", ic)
			}
			assertArgv(t, "InferenceConfig.Command", ic.Command.Args(), tc.argv)

			if tc.argv[0] != tc.bin {
				t.Errorf("argv[0] = %q, want the detection binary %q", tc.argv[0], tc.bin)
			}
		})
	}
}

// TestNativeACPAgents_LaunchArgvIsNotAliased guards the shared spec against
// handing every surface the same backing array — appending to one command's
// argv must not corrupt the next caller's.
func TestNativeACPAgents_LaunchArgvIsNotAliased(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			ag := tc.newAgent()
			ag.BuildCommand(CommandOptions{}).Args()[0] = "corrupted"
			assertArgv(t, "BuildCommand after mutation", ag.BuildCommand(CommandOptions{}).Args(), tc.argv)
			assertArgv(t, "Runtime.Cmd after mutation", ag.Runtime().Cmd.Args(), tc.argv)
			ia, ok := ag.(InferenceAgent)
			if !ok {
				t.Fatalf("%s does not implement InferenceAgent", tc.id)
			}
			assertArgv(t, "InferenceConfig.Command after mutation", ia.InferenceConfig().Command.Args(), tc.argv)
		})
	}
}

func TestNativeACPAgents_Runtime(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.newAgent().Runtime()
			if rt == nil {
				t.Fatal("Runtime() = nil")
			}
			if rt.Protocol != agent.ProtocolACP {
				t.Errorf("Protocol = %q, want %q", rt.Protocol, agent.ProtocolACP)
			}
			assertString(t, "WorkingDir", rt.WorkingDir, "{workspace}")
			if rt.Env == nil || len(rt.Env) != 0 {
				t.Errorf("Env = %v, want empty non-nil map", rt.Env)
			}
			if rt.ResourceLimits != DefaultResourceLimits {
				t.Errorf("ResourceLimits = %+v, want %+v", rt.ResourceLimits, DefaultResourceLimits)
			}
			// nil and empty RequiredEnv are different: an explicitly empty
			// slice documents "auth is optional, not a launch prerequisite".
			if !reflect.DeepEqual(rt.RequiredEnv, tc.requiredEnv) {
				t.Errorf("RequiredEnv = %#v, want %#v", rt.RequiredEnv, tc.requiredEnv)
			}
			assertString(t, "ProjectSkillDir", rt.ProjectSkillDir, tc.projectSkillDir)
			assertString(t, "UserSkillDir", rt.UserSkillDir, tc.userSkillDir)
			if rt.ProjectMCPStrategy != tc.projectMCPStrategy {
				t.Errorf("ProjectMCPStrategy = %#v, want %#v", rt.ProjectMCPStrategy, tc.projectMCPStrategy)
			}
			if rt.AssumeMcpSse != tc.assumeMcpSse || rt.AssumeMcpHttp != tc.assumeMcpHttp {
				t.Errorf("AssumeMcp{Sse,Http} = (%v, %v), want (%v, %v)",
					rt.AssumeMcpSse, rt.AssumeMcpHttp, tc.assumeMcpSse, tc.assumeMcpHttp)
			}
			if !slices.Equal(rt.StripEnv, tc.stripEnv) {
				t.Errorf("StripEnv = %#v, want %#v", rt.StripEnv, tc.stripEnv)
			}
		})
	}
}

func TestNativeACPAgents_SessionConfig(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			sc := tc.newAgent().Runtime().SessionConfig
			if !sc.NativeSessionResume {
				t.Error("NativeSessionResume = false, want true")
			}
			if !sc.SupportsRecovery() {
				t.Error("SupportsRecovery() = false, want true")
			}
			if sc.CanRecover == nil || !*sc.CanRecover {
				t.Errorf("CanRecover = %v, want explicit true", sc.CanRecover)
			}
			assertString(t, "SessionDirTemplate", sc.SessionDirTemplate, tc.sessionDirTemplate)
			assertString(t, "SessionDirTarget", sc.SessionDirTarget, tc.sessionDirTarget)
			if sc.NewSessionOnWorkspaceRebind != tc.newSessionOnWorkspaceRebind {
				t.Errorf("NewSessionOnWorkspaceRebind = %v, want %v",
					sc.NewSessionOnWorkspaceRebind, tc.newSessionOnWorkspaceRebind)
			}
		})
	}
}

func TestNativeACPAgents_Passthrough(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			pa, ok := tc.newAgent().(PassthroughAgent)
			if ok != tc.wantPassthrough {
				t.Fatalf("implements PassthroughAgent = %v, want %v", ok, tc.wantPassthrough)
			}
			if !tc.wantPassthrough {
				return
			}
			cfg := pa.PassthroughConfig()
			if !cfg.Supported {
				t.Error("PassthroughConfig.Supported = false, want true")
			}
			assertString(t, "Label", cfg.Label, passthroughLabel)
			assertString(t, "Description", cfg.Description, passthroughDescription)
			assertArgv(t, "PassthroughCmd", cfg.PassthroughCmd.Args(), []string{tc.bin})
			assertArgv(t, "ModelFlag", cfg.ModelFlag.Args(), tc.modelFlag)
			assertArgv(t, "ResumeFlag", cfg.ResumeFlag.Args(), tc.resumeFlag)
			assertArgv(t, "SessionResumeFlag", cfg.SessionResumeFlag.Args(), tc.sessionResumeFlag)
			if cfg.MCPStrategy != tc.mcpStrategy {
				t.Errorf("MCPStrategy = %#v, want %#v", cfg.MCPStrategy, tc.mcpStrategy)
			}
			if cfg.IdleTimeout != 3*time.Second {
				t.Errorf("IdleTimeout = %v, want 3s", cfg.IdleTimeout)
			}
			if cfg.BufferMaxBytes != DefaultBufferMaxBytes {
				t.Errorf("BufferMaxBytes = %d, want %d", cfg.BufferMaxBytes, DefaultBufferMaxBytes)
			}
		})
	}
}

// TestNativeACPAgents_PassthroughCommandUsesAgentPermSettings pins that the
// shared constructor wires each agent's own PermissionSettings into
// StandardPassthrough — Cursor's --force is the only cli_flag in the cluster,
// so a mis-wired shared default would silently drop or leak it.
func TestNativeACPAgents_PassthroughCommandUsesAgentPermSettings(t *testing.T) {
	values := map[string]bool{PermissionKeyCursorForce: true}
	for _, tc := range nativeACPCases {
		if !tc.wantPassthrough {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			pa := tc.newAgent().(PassthroughAgent)
			argv := pa.BuildPassthroughCommand(PassthroughOptions{PermissionValues: values}).Args()
			wantForce := slices.Contains(tc.permSettingKeys, PermissionKeyCursorForce)
			if got := slices.Contains(argv, "--force"); got != wantForce {
				t.Errorf("passthrough argv %#v contains --force = %v, want %v", argv, got, wantForce)
			}
		})
	}
}

func TestNativeACPAgents_PermissionSettings(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := tc.newAgent().PermissionSettings()
			if len(settings) != len(tc.permSettingKeys) {
				t.Fatalf("PermissionSettings() has %d entries %v, want %d %v",
					len(settings), keysOf(settings), len(tc.permSettingKeys), tc.permSettingKeys)
			}
			for _, key := range tc.permSettingKeys {
				if _, ok := settings[key]; !ok {
					t.Errorf("PermissionSettings() missing %q", key)
				}
			}
		})
	}
}

func TestNativeACPAgents_InstallScriptAndRemoteAuth(t *testing.T) {
	for _, tc := range nativeACPCases {
		t.Run(tc.name, func(t *testing.T) {
			ag := tc.newAgent()
			assertString(t, "InstallScript", ag.InstallScript(), tc.installScript)

			auth := ag.RemoteAuth()
			if (auth != nil) != tc.wantRemoteAuth {
				t.Fatalf("RemoteAuth() = %+v, want non-nil=%v", auth, tc.wantRemoteAuth)
			}
			if auth == nil {
				return
			}
			gotTypes := make([]string, 0, len(auth.Methods))
			for _, m := range auth.Methods {
				gotTypes = append(gotTypes, m.Type)
			}
			assertArgv(t, "RemoteAuth method types", gotTypes, tc.remoteAuthTypes)
		})
	}
}

func assertString(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", label, got, want)
	}
}

func assertArgv(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %#v, want %#v", label, got, want)
	}
}

func keysOf(m map[string]PermissionSetting) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
