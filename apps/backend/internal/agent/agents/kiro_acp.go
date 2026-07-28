package agents

import (
	_ "embed"
)

//go:embed logos/kiro_acp_light.svg
var kiroACPLogoLight []byte

//go:embed logos/kiro_acp_dark.svg
var kiroACPLogoDark []byte

const kiroACPBin = "kiro-cli-chat"

var (
	_ Agent            = (*KiroACP)(nil)
	_ PassthroughAgent = (*KiroACP)(nil)
	_ InferenceAgent   = (*KiroACP)(nil)
)

// KiroACP implements Agent for AWS Kiro using ACP. The CLI binary
// (kiro-cli-chat) is installed via AWS-provided tooling.
type KiroACP struct {
	nativeACPPassthroughAgent
}

func NewKiroACP() *KiroACP {
	return &KiroACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "kiro-acp",
			name:          "Kiro ACP Agent",
			displayName:   "Kiro",
			description:   "AWS Kiro coding agent using the ACP protocol via kiro-cli-chat.",
			displayOrder:  15,
			bin:           kiroACPBin,
			args:          []string{"acp"},
			logoLight:     kiroACPLogoLight,
			logoDark:      kiroACPLogoDark,
			installScript: "Install Kiro CLI from AWS",
			permSettings:  emptyPermSettings,
			runtime: nativeACPRuntimeSpec{
				userSkillDir: ".kiro/skills",
				// Verified against Kiro CLI 2.15.0 by driving `kiro-cli-chat acp`
				// through session/new: each ACP session is persisted as
				// ~/.kiro/sessions/cli/<sessionId>.{json,jsonl}. KIRO_HOME
				// relocates the directory. Note ~/.kiro carries config + sessions
				// only — the login token lives in the separate
				// ~/.local/share/kiro-cli/data.sqlite3 (auth_kv table), so this
				// mount persists resumable sessions but not authentication.
				sessionDirTemplate: "{home}/.kiro",
			},
		},
		nativeACPPassthroughSpec{modelFlag: nativeACPModelFlag()},
	)}
}
