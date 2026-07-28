package agents

import (
	_ "embed"
)

//go:embed logos/kimi_acp_light.svg
var kimiACPLogoLight []byte

//go:embed logos/kimi_acp_dark.svg
var kimiACPLogoDark []byte

const kimiACPBin = "kimi"

var (
	_ Agent            = (*KimiACP)(nil)
	_ PassthroughAgent = (*KimiACP)(nil)
	_ InferenceAgent   = (*KimiACP)(nil)
)

// KimiACP implements Agent for Moonshot's Kimi CLI using ACP.
// Not on npm — users must install the kimi binary from Moonshot AI.
type KimiACP struct {
	nativeACPPassthroughAgent
}

func NewKimiACP() *KimiACP {
	return &KimiACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "kimi-acp",
			name:          "Kimi ACP Agent",
			displayName:   "Kimi",
			description:   "Moonshot AI Kimi coding agent using the ACP protocol over stdin/stdout.",
			displayOrder:  14,
			bin:           kimiACPBin,
			args:          []string{"acp"},
			logoLight:     kimiACPLogoLight,
			logoDark:      kimiACPLogoDark,
			installScript: "Install Kimi CLI from Moonshot AI",
			permSettings:  emptyPermSettings,
			runtime: nativeACPRuntimeSpec{
				sessionDirTemplate: "{home}/.kimi",
			},
		},
		nativeACPPassthroughSpec{modelFlag: nativeACPModelFlag()},
	)}
}
