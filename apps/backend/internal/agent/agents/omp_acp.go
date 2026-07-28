package agents

import (
	_ "embed"
)

//go:embed logos/omp_acp_light.svg
var ompACPLogoLight []byte

//go:embed logos/omp_acp_dark.svg
var ompACPLogoDark []byte

const ompACPBin = "omp"

var (
	_ Agent            = (*OmpACP)(nil)
	_ PassthroughAgent = (*OmpACP)(nil)
	_ InferenceAgent   = (*OmpACP)(nil)
)

// OmpACP implements Agent for the Oh My Pi (omp) coding agent via its native
// `omp acp` subcommand. Distributed as a single binary (bun-installed); BYO
// API key — omp reads any of the dozen provider env vars (ANTHROPIC_API_KEY,
// OPENAI_API_KEY, GEMINI_API_KEY, ...) directly.
type OmpACP struct {
	nativeACPPassthroughAgent
}

func NewOmpACP() *OmpACP {
	return &OmpACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "omp-acp",
			name:          "Oh My Pi ACP Agent",
			displayName:   ompACPBin,
			description:   "Oh My Pi (omp) coding agent using the ACP protocol via the `omp acp` subcommand.",
			displayOrder:  18,
			bin:           ompACPBin,
			args:          []string{"acp"},
			logoLight:     ompACPLogoLight,
			logoDark:      ompACPLogoDark,
			installScript: "bun install -g @oh-my-pi/pi-coding-agent",
			permSettings:  emptyPermSettings,
			runtime: nativeACPRuntimeSpec{
				projectSkillDir:    ".omp/skills",
				userSkillDir:       ".omp/agent/skills",
				sessionDirTemplate: "{home}/.omp",
			},
		},
		nativeACPPassthroughSpec{
			modelFlag:         nativeACPModelFlag(),
			resumeFlag:        NewParam("-c"),
			sessionResumeFlag: NewParam("--resume"),
		},
	)}
}
