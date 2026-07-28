package agents

import (
	_ "embed"
)

//go:embed logos/devin_acp_light.svg
var devinACPLogoLight []byte

//go:embed logos/devin_acp_dark.svg
var devinACPLogoDark []byte

const devinACPBin = "devin"

const (
	devinCredentialsDir     = ".local/share/devin"
	devinCredentialsRelPath = devinCredentialsDir + "/credentials.toml"
	devinDefaultAPIServer   = "https://server.self-serve.windsurf.com"
)

const devinACPInstallScript = `set -e
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL https://cli.devin.ai/install.sh -o "$tmp"
if ! bash "$tmp" && [ ! -x "$HOME/.local/bin/devin" ]; then
  exit 1
fi
export PATH="$HOME/.local/bin:$PATH"
persist_devin_path() {
  grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$1" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$1"
}
persist_devin_path "$HOME/.profile"
persist_devin_path "$HOME/.bash_profile"
persist_devin_path "$HOME/.bashrc"
persist_devin_path "$HOME/.zprofile"
persist_devin_path "$HOME/.zshrc"
devin --version >/dev/null`

const devinACPCredentialsSetupScript = `mkdir -p "${HOME}/.local/share/devin"
escape_toml() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}
api_key="$(escape_toml "${WINDSURF_API_KEY}")"
api_server="$(escape_toml "${WINDSURF_API_SERVER_URL:-` + devinDefaultAPIServer + `}")"
umask 077
cat > "${HOME}/.local/share/devin/credentials.toml" <<CREDS
windsurf_api_key = "${api_key}"
api_server_url = "${api_server}"
CREDS
chmod 600 "${HOME}/.local/share/devin/credentials.toml"`

var (
	_ Agent            = (*DevinACP)(nil)
	_ PassthroughAgent = (*DevinACP)(nil)
	_ InferenceAgent   = (*DevinACP)(nil)
)

// DevinACP implements Agent for Cognition's Devin CLI using ACP.
// The CLI binary (devin) is installed via the Devin Desktop app or
// standalone installer. It speaks ACP natively via the `devin acp` subcommand.
//
// Credential handling: `devin acp` checks the ACP_BACKEND environment variable.
// When set (e.g. by Windsurf Next), it requires the ACP host to call
// `authenticate` and refuses local credentials. When unset, it falls back to
// reading ~/.local/share/devin/credentials.toml directly. The process manager
// strips ACP_BACKEND from the child environment so Devin uses the fall-back
// path — no protocol-level authenticate needed.
type DevinACP struct {
	nativeACPPassthroughAgent
}

func NewDevinACP() *DevinACP {
	return &DevinACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "devin-acp",
			name:          "Devin ACP Agent",
			displayName:   "Devin",
			description:   "Cognition Devin coding agent using the ACP protocol via `devin acp`.",
			displayOrder:  19,
			bin:           devinACPBin,
			args:          []string{"acp"},
			logoLight:     devinACPLogoLight,
			logoDark:      devinACPLogoDark,
			installScript: devinACPInstallScript,
			permSettings:  emptyPermSettings,
			remoteAuth: &RemoteAuth{
				Methods: []RemoteAuthMethod{
					{
						Type:  "files",
						Label: "Copy Devin CLI credentials",
						SourceFiles: map[string][]string{
							"darwin": {devinCredentialsRelPath},
							"linux":  {devinCredentialsRelPath},
						},
						TargetRelDir: devinCredentialsDir,
					},
					{
						Type:        "env",
						EnvVar:      "WINDSURF_API_KEY",
						SetupHint:   "Set WINDSURF_API_KEY to authenticate Devin CLI in remote or headless environments.",
						SetupScript: devinACPCredentialsSetupScript,
					},
				},
			},
			runtime: nativeACPRuntimeSpec{
				// Devin advertises mcpCapabilities {http: false, sse: false} but
				// actually supports streamable HTTP MCP (used by Windsurf Next).
				// Without these overrides, filterMcpServersByCapabilities drops the
				// Kandev MCP server (exposed via HTTP /mcp and SSE /sse), and the
				// agent loses access to task tools (get_task_plan_kandev, etc.).
				assumeMcpSse:  true,
				assumeMcpHttp: true,
				// See DevinACP doc comment for the ACP_BACKEND rationale.
				stripEnv:           []string{"ACP_BACKEND"},
				sessionDirTemplate: "{home}/.local/share/devin",
				sessionDirTarget:   "/root/.local/share/devin",
			},
		},
		// Devin's CLI takes no --model flag in passthrough mode.
		nativeACPPassthroughSpec{},
	)}
}
