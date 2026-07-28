package agents

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

// Native-binary ACP agents (Cursor, Kimi, Kiro, Qoder, Trae, Omp, Devin,
// Grok) share one scaffold: a CLI distributed outside npm that speaks ACP
// over stdin/stdout on a fixed argv, resumes sessions natively, and reports
// availability by looking for its binary on PATH. nativeACPSpec captures
// everything that differs between them; nativeACPAgent turns a spec into the
// Agent implementation, so each agent file is just its own values.

// passthroughLabel and passthroughDescription are the shared UI strings for
// the raw-terminal affordance every native-binary ACP agent exposes.
const (
	passthroughLabel       = "CLI Passthrough"
	passthroughDescription = "Show terminal directly instead of chat interface"
)

// workspaceWorkingDir is the RuntimeConfig.WorkingDir placeholder the
// lifecycle expands to the session's materialized workspace path.
const workspaceWorkingDir = "{workspace}"

// nativeACPIdleTimeout is the passthrough idle window after which the PTY is
// considered quiescent.
const nativeACPIdleTimeout = 3 * time.Second

// nativeACPModelFlag is the `--model <model>` passthrough template shared by
// the native-binary CLIs. Returned fresh per call so no two agents alias the
// same backing array.
func nativeACPModelFlag() Param { return NewParam("--model", "{model}") }

// nativeACPRuntimeSpec holds the RuntimeConfig fields that vary between
// native-binary ACP agents. Everything omitted here is identical across the
// cluster and is filled in by nativeACPAgent.Runtime.
type nativeACPRuntimeSpec struct {
	// requiredEnv is a launch prerequisite list. An explicitly empty (non-nil)
	// slice documents "auth is optional", which is distinct from unset.
	requiredEnv []string
	// projectSkillDir / userSkillDir are the CWD- and home-relative skill
	// directories this CLI reads.
	projectSkillDir string
	userSkillDir    string
	// projectMCPStrategy materializes MCP servers into a project-local config
	// for CLIs whose ACP mode has no MCP flag or env var.
	projectMCPStrategy mcpconfig.PassthroughMCPStrategy
	// assumeMcpSse / assumeMcpHttp override an agent's advertised MCP
	// capabilities when the advertisement understates what it supports.
	assumeMcpSse  bool
	assumeMcpHttp bool
	// stripEnv lists env vars removed from the child process entirely.
	stripEnv []string
	// sessionDirTemplate / sessionDirTarget locate the CLI's session state so
	// the Docker runtime can mount it. An empty template means resume has no
	// persistence across container restarts.
	sessionDirTemplate string
	sessionDirTarget   string
	// newSessionOnWorkspaceRebind forces session/new when an idle execution's
	// working directory changes.
	newSessionOnWorkspaceRebind bool
}

// nativeACPSpec is the complete per-agent description of a native-binary ACP
// agent.
type nativeACPSpec struct {
	id           string
	name         string
	displayName  string
	description  string
	displayOrder int
	// bin is the executable looked up on PATH by IsInstalled, and the command
	// used for raw passthrough.
	bin string
	// args are the arguments appended to bin to launch ACP mode. BuildCommand,
	// Runtime().Cmd and InferenceConfig().Command all use the same argv.
	args          []string
	logoLight     []byte
	logoDark      []byte
	installScript string
	remoteAuth    *RemoteAuth
	permSettings  map[string]PermissionSetting
	runtime       nativeACPRuntimeSpec
}

// launchArgv returns the full ACP launch command line.
func (s nativeACPSpec) launchArgv() []string {
	argv := make([]string, 0, len(s.args)+1)
	argv = append(argv, s.bin)
	return append(argv, s.args...)
}

// nativeACPAgent implements Agent and InferenceAgent from a nativeACPSpec.
// Agents embed it (directly, or via nativeACPPassthroughAgent) and override
// only the methods their spec cannot express.
type nativeACPAgent struct {
	spec nativeACPSpec
}

func (a *nativeACPAgent) ID() string          { return a.spec.id }
func (a *nativeACPAgent) Name() string        { return a.spec.name }
func (a *nativeACPAgent) DisplayName() string { return a.spec.displayName }
func (a *nativeACPAgent) Description() string { return a.spec.description }
func (a *nativeACPAgent) Enabled() bool       { return true }
func (a *nativeACPAgent) DisplayOrder() int   { return a.spec.displayOrder }

func (a *nativeACPAgent) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return a.spec.logoDark
	}
	return a.spec.logoLight
}

func (a *nativeACPAgent) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(a.spec.bin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *nativeACPAgent) BuildCommand(opts CommandOptions) Command {
	return Cmd(a.spec.launchArgv()...).Build()
}

func (a *nativeACPAgent) Runtime() *RuntimeConfig {
	canRecover := true
	rt := a.spec.runtime
	return &RuntimeConfig{
		Cmd:                Cmd(a.spec.launchArgv()...).Build(),
		WorkingDir:         workspaceWorkingDir,
		Env:                map[string]string{},
		RequiredEnv:        rt.requiredEnv,
		ResourceLimits:     DefaultResourceLimits,
		Protocol:           agent.ProtocolACP,
		ProjectSkillDir:    rt.projectSkillDir,
		UserSkillDir:       rt.userSkillDir,
		ProjectMCPStrategy: rt.projectMCPStrategy,
		AssumeMcpSse:       rt.assumeMcpSse,
		AssumeMcpHttp:      rt.assumeMcpHttp,
		StripEnv:           rt.stripEnv,
		SessionConfig: SessionConfig{
			NativeSessionResume:         true,
			NewSessionOnWorkspaceRebind: rt.newSessionOnWorkspaceRebind,
			CanRecover:                  &canRecover,
			SessionDirTemplate:          rt.sessionDirTemplate,
			SessionDirTarget:            rt.sessionDirTarget,
		},
	}
}

func (a *nativeACPAgent) RemoteAuth() *RemoteAuth { return a.spec.remoteAuth }

func (a *nativeACPAgent) InstallScript() string { return a.spec.installScript }

func (a *nativeACPAgent) PermissionSettings() map[string]PermissionSetting {
	return a.spec.permSettings
}

func (a *nativeACPAgent) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(a.spec.launchArgv()...),
	}
}

func (a *nativeACPAgent) BillingType() usage.BillingType { return defaultBillingType() }

// nativeACPPassthroughSpec declares the per-agent parts of the shared
// terminal-passthrough config. A zero value is valid: it means the CLI takes
// no model or resume flags in passthrough mode.
type nativeACPPassthroughSpec struct {
	modelFlag         Param
	resumeFlag        Param
	sessionResumeFlag Param
	mcpStrategy       mcpconfig.PassthroughMCPStrategy
}

// nativeACPPassthroughAgent is a nativeACPAgent that also runs its CLI in raw
// terminal passthrough mode. Grok deliberately stays on the bare
// nativeACPAgent so it does not satisfy PassthroughAgent (see ADR 0014).
type nativeACPPassthroughAgent struct {
	nativeACPAgent
	StandardPassthrough
}

func newNativeACPPassthrough(spec nativeACPSpec, pt nativeACPPassthroughSpec) nativeACPPassthroughAgent {
	return nativeACPPassthroughAgent{
		nativeACPAgent: nativeACPAgent{spec: spec},
		StandardPassthrough: StandardPassthrough{
			PermSettings: spec.permSettings,
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             passthroughLabel,
				Description:       passthroughDescription,
				PassthroughCmd:    NewCommand(spec.bin),
				ModelFlag:         pt.modelFlag,
				ResumeFlag:        pt.resumeFlag,
				SessionResumeFlag: pt.sessionResumeFlag,
				IdleTimeout:       nativeACPIdleTimeout,
				BufferMaxBytes:    DefaultBufferMaxBytes,
				MCPStrategy:       pt.mcpStrategy,
			},
		},
	}
}
