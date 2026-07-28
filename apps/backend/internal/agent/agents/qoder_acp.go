package agents

import (
	_ "embed"
)

//go:embed logos/qoder_acp_light.svg
var qoderACPLogoLight []byte

//go:embed logos/qoder_acp_dark.svg
var qoderACPLogoDark []byte

const qoderACPBin = "qodercli"

var (
	_ Agent            = (*QoderACP)(nil)
	_ PassthroughAgent = (*QoderACP)(nil)
	_ InferenceAgent   = (*QoderACP)(nil)
)

// QoderACP implements Agent for the Qoder CLI using ACP.
type QoderACP struct {
	nativeACPPassthroughAgent
}

func NewQoderACP() *QoderACP {
	return &QoderACP{newNativeACPPassthrough(
		nativeACPSpec{
			id:            "qoder-acp",
			name:          "Qoder ACP Agent",
			displayName:   "Qoder",
			description:   "Qoder coding agent using the ACP protocol via qodercli --acp.",
			displayOrder:  16,
			bin:           qoderACPBin,
			args:          []string{"--acp"},
			logoLight:     qoderACPLogoLight,
			logoDark:      qoderACPLogoDark,
			installScript: "Install Qoder CLI from https://qoder.com",
			permSettings:  emptyPermSettings,
			runtime: nativeACPRuntimeSpec{
				// Verified against qodercli 1.1.7: every user-level artifact
				// (settings.json, .auth/, logs/sessions/<project>) is written
				// under ~/.qoder, and the --config-dir flag relocates that single
				// root wholesale — running with it leaves nothing else in $HOME.
				sessionDirTemplate: "{home}/.qoder",
			},
		},
		nativeACPPassthroughSpec{modelFlag: nativeACPModelFlag()},
	)}
}
