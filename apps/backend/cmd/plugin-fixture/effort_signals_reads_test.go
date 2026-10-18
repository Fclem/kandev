package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

type effortTestHost struct {
	*fakeHost
	active int64
}
type effortSessionReader struct {
	pluginsdk.SessionReader
	host *effortTestHost
}

func (h *effortTestHost) Sessions() pluginsdk.SessionReader { return effortSessionReader{host: h} }
func (r effortSessionReader) List(context.Context, pluginsdk.SessionFilter, pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
	return []pluginsdk.Session{{ID: "session-one", TaskID: "task-one"}}, nil, nil
}
func (r effortSessionReader) CodeStats(context.Context, pluginsdk.SessionFilter, pluginsdk.Page) ([]pluginsdk.SessionCodeStats, *pluginsdk.PageInfo, error) {
	return []pluginsdk.SessionCodeStats{{SessionID: "session-one", LinesAddedCommitted: 20}}, nil, nil
}
func (r effortSessionReader) UserActivity(context.Context, pluginsdk.SessionActivityFilter, pluginsdk.Page) ([]pluginsdk.SessionUserActivity, *pluginsdk.PageInfo, error) {
	return []pluginsdk.SessionUserActivity{{SessionID: "session-one", TaskID: "task-one", ActiveMS: r.host.active, ActivityMeasured: true}}, nil, nil
}
func (r effortSessionReader) Turns(context.Context, pluginsdk.SessionActivityFilter, pluginsdk.Page) ([]pluginsdk.SessionTurn, *pluginsdk.PageInfo, error) {
	return []pluginsdk.SessionTurn{{ID: "turn-one", SessionID: "session-one", TaskID: "task-one"}}, nil, nil
}

type effortTaskReader struct{ pluginsdk.TaskReader }

func (h *effortTestHost) Tasks() pluginsdk.TaskReader { return effortTaskReader{} }
func (effortTaskReader) Get(context.Context, string) (*pluginsdk.Task, error) {
	return &pluginsdk.Task{ID: "task-one", WorkspaceID: "workspace-one"}, nil
}

type effortMessageReader struct{ pluginsdk.MessageReader }

func (h *effortTestHost) Messages() pluginsdk.MessageReader { return effortMessageReader{} }
func (effortMessageReader) List(_ context.Context, f pluginsdk.MessageFilter, _ pluginsdk.Page) ([]pluginsdk.Message, *pluginsdk.PageInfo, error) {
	return []pluginsdk.Message{{Origin: "human"}, {Origin: "workflow"}}, nil, nil
}

type effortExactReader struct{ pluginsdk.ExactQueryManager }

func (h *effortTestHost) ExactQueries() pluginsdk.ExactQueryManager { return effortExactReader{} }
func (effortExactReader) ListChangeRequestFeedback(context.Context, pluginsdk.ExactChangeRequestFeedbackQuery) (pluginsdk.ExactChangeRequestFeedbackPage, error) {
	return pluginsdk.ExactChangeRequestFeedbackPage{Items: []pluginsdk.ChangeRequestFeedbackTask{{TaskID: "task-one", Items: []pluginsdk.ChangeRequestFeedbackItem{{Body: "private feedback", AuthorLogin: "private-login"}}}}}, nil
}
func (effortExactReader) ListTaskUsage(context.Context, pluginsdk.ExactTaskUsageQuery) (pluginsdk.ExactTaskUsagePage, error) {
	return pluginsdk.ExactTaskUsagePage{Items: []pluginsdk.TaskUsageObservation{{TaskID: "task-one", TokensTotal: 123}}}, nil
}

func TestEffortSignalsFreshReadAndDerivedState(t *testing.T) {
	host := &effortTestHost{fakeHost: &fakeHost{}, active: 15000}
	plugin := newFixturePluginAt(t.TempDir())
	plugin.SetHost(host)
	request := &pluginsdk.PluginActionRequest{ActionKey: "estimate", Context: pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-one", TaskID: "task-one"}}
	response, err := plugin.HandleAction(context.Background(), request)
	require.NoError(t, err)
	var first fixtureEffortSummary
	require.NoError(t, json.Unmarshal(response.Body, &first))
	require.Equal(t, int64(15000), first.ActiveMS)
	require.Equal(t, 1, first.HumanPrompts)
	require.Len(t, first.Turns, 1)
	require.Equal(t, int64(20), first.Code[0].LinesAddedCommitted)
	require.Equal(t, int64(123), first.Usage[0].TokensTotal)
	host.active = 30000
	request.ActionKey = "summary"
	response, err = plugin.HandleAction(context.Background(), request)
	require.NoError(t, err)
	var second fixtureEffortSummary
	require.NoError(t, json.Unmarshal(response.Body, &second))
	require.Equal(t, int64(30000), second.ActiveMS, "summary must read Host, not the stored estimate")
	require.Len(t, host.setStateCalls, 1)
	persisted, err := json.Marshal(host.setStateCalls[0].value)
	require.NoError(t, err)
	require.NotContains(t, string(persisted), "private feedback")
	require.NotContains(t, string(persisted), "private-login")
}
