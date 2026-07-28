package agents

import (
	_ "embed"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

//go:embed logos/cursor_acp_light.svg
var cursorACPLogoLight []byte

//go:embed logos/cursor_acp_dark.svg
var cursorACPLogoDark []byte

const cursorACPBin = "cursor-agent"

// cursorACPInstallScript installs cursor-agent, which isn't on npm. The
// official installer drops the binary into ~/.local/bin; make sure that dir is
// on PATH for the rest of the prepare script and for future shells on the
// sprite.
const cursorACPInstallScript = `set -e
tmp="$(mktemp)"
curl -fsS https://cursor.com/install -o "$tmp"
bash "$tmp"
rm -f "$tmp"
export PATH="$HOME/.local/bin:$PATH"
grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.bashrc" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"`

// cursorPermSettings maps a curated CLI flag to cursor-agent's --force switch.
// In ACP mode Cursor emits session/request_permission for commands off its
// allowlist; --force ("Run Everything") suppresses those prompts. Default off
// because --force runs everything unsandboxed. Universal agentctl auto-approve
// (PermissionKeyAutoApprove) is added by CatalogPermissionSettings, not here.
var cursorPermSettings = map[string]PermissionSetting{
	PermissionKeyCursorForce: {
		Supported:   true,
		Default:     false,
		Label:       "Cursor run everything (--force)",
		Description: "Append cursor-agent --force so the CLI stops prompting for non-allowlisted commands (unsandboxed).",
		ApplyMethod: PermissionApplyMethodCLIFlag,
		CLIFlag:     "--force",
	},
}

var (
	_ Agent            = (*CursorACP)(nil)
	_ PassthroughAgent = (*CursorACP)(nil)
	_ InferenceAgent   = (*CursorACP)(nil)
)

// CursorACP implements Agent for Cursor's CLI via its native ACP mode.
// Cursor isn't published to npm — users must install the cursor-agent binary
// from Cursor (Pro subscription required).
//
// --force is applied via profile cli_flags (seeded from cursor_force), not
// PermissionValues, so auto_approve stays agentctl-only.
type CursorACP struct {
	nativeACPPassthroughAgent
}

func NewCursorACP() *CursorACP {
	return &CursorACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "cursor-acp",
			name:          "Cursor ACP Agent",
			displayName:   "Cursor",
			description:   "Cursor CLI coding agent (cursor-agent) using the ACP protocol. Requires a Cursor Pro subscription.",
			displayOrder:  13,
			bin:           cursorACPBin,
			args:          []string{"acp"},
			logoLight:     cursorACPLogoLight,
			logoDark:      cursorACPLogoDark,
			installScript: cursorACPInstallScript,
			permSettings:  cursorPermSettings,
			remoteAuth: &RemoteAuth{
				Methods: []RemoteAuthMethod{
					{
						Type:      "env",
						EnvVar:    "CURSOR_API_KEY",
						SetupHint: "Create an API key at https://cursor.com/dashboard/integrations (Cursor Pro).",
					},
				},
			},
			runtime: nativeACPRuntimeSpec{
				userSkillDir:       ".cursor/skills",
				sessionDirTemplate: "{home}/.cursor",
				// cursor-agent has no MCP flag/env; write or merge a
				// project-local .cursor/mcp.json into the worktree.
				projectMCPStrategy: mcpconfig.CursorStrategy{},
			},
		},
		nativeACPPassthroughSpec{
			modelFlag:   nativeACPModelFlag(),
			mcpStrategy: mcpconfig.CursorStrategy{},
		},
	)}
}
