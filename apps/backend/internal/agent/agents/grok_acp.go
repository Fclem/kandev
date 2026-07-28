package agents

import (
	_ "embed"
)

//go:embed logos/grok_acp_light.svg
var grokACPLogoLight []byte

//go:embed logos/grok_acp_dark.svg
var grokACPLogoDark []byte

const grokACPBin = "grok"

// grokACPArgs are the ACP / inference launch arguments. --no-auto-update must
// precede the agent subcommand so background self-update checks do not run
// for Kandev-managed sessions.
var grokACPArgs = []string{"--no-auto-update", "agent", "stdio"}

var (
	_ Agent          = (*GrokACP)(nil)
	_ InferenceAgent = (*GrokACP)(nil)
	_ LoginAgent     = (*GrokACP)(nil)
)

// GrokACP implements Agent for xAI's Grok Build CLI using native ACP over
// stdin/stdout. Raw TUI passthrough is intentionally deferred (requires a
// project-scoped PassthroughMCPStrategy — see ADR 0014), so this embeds the
// bare nativeACPAgent rather than the passthrough variant.
type GrokACP struct {
	nativeACPAgent
}

func NewGrokACP() *GrokACP {
	return &GrokACP{nativeACPAgent{spec: nativeACPSpec{
		id:            "grok-acp",
		name:          "Grok ACP Agent",
		displayName:   "Grok",
		description:   "xAI Grok coding agent using the ACP protocol over stdin/stdout.",
		displayOrder:  20,
		bin:           grokACPBin,
		args:          grokACPArgs,
		logoLight:     grokACPLogoLight,
		logoDark:      grokACPLogoDark,
		installScript: "npm install -g @xai-official/grok",
		permSettings:  emptyPermSettings,
		remoteAuth: &RemoteAuth{
			Methods: []RemoteAuthMethod{
				{
					Type:  "files",
					Label: "Copy auth files",
					SourceFiles: map[string][]string{
						"darwin": {".grok/auth.json"},
						"linux":  {".grok/auth.json"},
					},
					// Only auth.json — do not copy config.toml, sessions, logs, or caches.
					TargetRelDir: ".grok",
				},
				{
					Type:   "env",
					EnvVar: "XAI_API_KEY",
				},
			},
		},
		runtime: nativeACPRuntimeSpec{
			// Cached OAuth or optional XAI_API_KEY — not a launch prerequisite.
			requiredEnv:                 []string{},
			projectSkillDir:             ".grok/skills",
			userSkillDir:                ".grok/skills",
			sessionDirTemplate:          "{home}/.grok",
			sessionDirTarget:            "/root/.grok",
			newSessionOnWorkspaceRebind: true,
		},
	}}}
}

// LoginCommand uses device-code auth so headless/container/SSH environments
// can complete sign-in without a local browser.
func (a *GrokACP) LoginCommand() *LoginCommand {
	return &LoginCommand{
		Cmd:         []string{grokACPBin, "login", "--device-auth"},
		Description: "Sign in with your xAI / Grok account.",
	}
}
