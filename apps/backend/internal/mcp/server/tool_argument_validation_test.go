package mcp

import (
	"encoding/json"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolArgumentValidationRejectsUnknownTopLevelArgument(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_workspaces_kandev", map[string]interface{}{
		"unexpected": "discarded today",
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	assert.NotEqual(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t,
		"invalid arguments for list_workspaces_kandev: validation failed at $ (keyword: additionalProperties)",
		content.Text)
}

func TestToolArgumentValidation(t *testing.T) {
	t.Run("accepts an empty object for a parameterless tool", func(t *testing.T) {
		backend := &testBackend{
			response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
		}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workspaces_kandev", map[string]interface{}{})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	})

	t.Run("accepts omitted arguments for a parameterless tool", func(t *testing.T) {
		backend := &testBackend{
			response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
		}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workspaces_kandev", nil)

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	})

	t.Run("rejects a missing required argument", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workflows_kandev", map[string]interface{}{})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("rejects the wrong declared type", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "create_task_kandev", map[string]interface{}{
			"title":       "Typed arguments",
			"start_agent": "false",
		})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("rejects a declared enum violation", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "message_task_kandev", map[string]interface{}{
			"task_id":       "task-target",
			"prompt":        "Status?",
			"delivery_mode": "later",
		})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("keeps an intentionally open nested map", func(t *testing.T) {
		backend := &testBackend{response: map[string]interface{}{"id": "profile-1"}}
		s := newTestServer(t, backend)

		result := callTool(t, s, "create_executor_profile_kandev", map[string]interface{}{
			"executor_id": "exec-local",
			"name":        "Custom",
			"config": map[string]interface{}{
				"provider_specific_key": "allowed",
			},
		})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPCreateExecutorProfile, backend.lastAction)
	})
}

func TestToolArgumentValidationDoesNotExposeRejectedValues(t *testing.T) {
	const (
		secret   = "api-key-super-secret-123"
		toolName = "secret_pattern_tool"
	)
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			toolName,
			"Validates a secret without exposing it.",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"token": {"type": "string", "pattern": "^safe$"}
				},
				"required": ["token"]
			}`),
		),
		s.wrapHandler(toolName, s.listWorkspacesHandler()),
	)
	s.rebuildToolArgumentValidators()

	result := callTool(t, s, toolName, map[string]interface{}{
		"token": secret,
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "/token")
	assert.Contains(t, content.Text, "pattern")
	assert.NotContains(t, content.Text, secret)
}

func TestAllRegisteredToolSchemasCompile(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeConfig, ModeExternal, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			log := newTestLogger(t)
			s := New(&testBackend{}, "session-1", "task-1", 10005, log, "", true, mode)
			tools := s.mcpServer.ListTools()

			s.validatorMu.RLock()
			defer s.validatorMu.RUnlock()
			require.Len(t, s.toolValidators, len(tools))
			for name := range tools {
				validator, ok := s.toolValidators[name]
				require.True(t, ok, "missing validator for %s", name)
				assert.NoError(t, validator.err, "schema for %s must compile", name)
				assert.NotNil(t, validator.schema, "schema for %s must compile", name)
			}
		})
	}
}

func TestSetModeRebuildsToolValidators(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "workflow-1"}}
	s := newTaskModeServer(t, backend, "task-current")

	s.SetMode(ModeConfig)
	result := callTool(t, s, "create_workflow_kandev", map[string]interface{}{
		"workspace_id": "workspace-1",
		"name":         "Validated workflow",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateWorkflow, backend.lastAction)
}

func TestToolValidationWaitsForModeChangeLock(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	req := mcp.CallToolRequest{}
	req.Method = "tools/call"
	req.Params.Name = "list_workspaces_kandev"
	req.Params.Arguments = map[string]interface{}{}

	s.mu.Lock()
	validationDone := make(chan error, 1)
	go func() {
		_, err := s.validateToolArguments("list_workspaces_kandev", req)
		validationDone <- err
	}()

	select {
	case err := <-validationDone:
		t.Fatalf("validation completed while a mode change held the write lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	s.mu.Unlock()

	assert.NoError(t, <-validationDone)
}
