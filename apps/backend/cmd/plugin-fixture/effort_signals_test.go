package main

import (
	"context"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-PLUGINS-EFFORT-SIGNALS-005.2
func TestEffortSignalsActionsRequireHost(t *testing.T) {
	for _, action := range []string{"summary", "estimate"} {
		t.Run(action, func(t *testing.T) {
			plugin := newFixturePluginAt(t.TempDir())
			_, err := plugin.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{ActionKey: action, Context: pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-one", TaskID: "task-one"}})
			require.EqualError(t, err, "plugin-fixture: host unavailable")
		})
	}
}
