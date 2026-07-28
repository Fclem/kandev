package agents

import (
	_ "embed"
)

//go:embed logos/trae_acp_light.svg
var traeACPLogoLight []byte

//go:embed logos/trae_acp_dark.svg
var traeACPLogoDark []byte

const traeACPBin = "traecli"

var (
	_ Agent            = (*TraeACP)(nil)
	_ PassthroughAgent = (*TraeACP)(nil)
	_ InferenceAgent   = (*TraeACP)(nil)
)

// TraeACP implements Agent for ByteDance's Trae IDE CLI using ACP.
type TraeACP struct {
	nativeACPPassthroughAgent
}

func NewTraeACP() *TraeACP {
	return &TraeACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "trae-acp",
			name:          "Trae ACP Agent",
			displayName:   "Trae",
			description:   "ByteDance Trae IDE coding agent using the ACP protocol via traecli acp serve.",
			displayOrder:  17,
			bin:           traeACPBin,
			args:          []string{"acp", "serve"},
			logoLight:     traeACPLogoLight,
			logoDark:      traeACPLogoDark,
			installScript: "Install Trae IDE CLI from https://trae.ai",
			permSettings:  emptyPermSettings,
			runtime: nativeACPRuntimeSpec{
				// Verified against Trae CLI 0.120.48 by driving `traecli acp
				// serve` through session/new + session/prompt: the ACP session ID
				// appears as ~/.cache/trae-cli/sessions/<sessionId>/, which is
				// where the CLI persists conversation history and metadata.
				// ~/.trae holds config only (traecli.yaml, skills, agents,
				// commands) and the login token lives in the OS keyring, not a
				// file, so neither belongs here. The published manual still
				// documents the older ~/.cache/coco/ cache root; the shipped
				// binary uses ~/.cache/trae-cli, so re-check this on upgrades.
				sessionDirTemplate: "{home}/.cache/trae-cli",
			},
		},
		nativeACPPassthroughSpec{modelFlag: nativeACPModelFlag()},
	)}
}
