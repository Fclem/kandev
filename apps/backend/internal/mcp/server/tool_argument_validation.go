package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"go.uber.org/zap"
)

type toolArgumentValidator struct {
	schema *jsonschema.Schema
	err    error
}

func (s *Server) rebuildToolArgumentValidators() {
	tools := s.mcpServer.ListTools()
	validators := make(map[string]toolArgumentValidator, len(tools))

	for name, serverTool := range tools {
		schema, err := compileToolArgumentSchema(name, serverTool.Tool)
		validators[name] = toolArgumentValidator{schema: schema, err: err}
		if err != nil {
			s.logger.Error("failed to compile MCP tool argument schema",
				zap.String("tool", name),
				zap.Error(err))
		}
	}

	s.validatorMu.Lock()
	s.toolValidators = validators
	s.validatorMu.Unlock()
}

func compileToolArgumentSchema(toolName string, tool mcp.Tool) (*jsonschema.Schema, error) {
	rawSchema := tool.RawInputSchema
	if rawSchema == nil {
		var err error
		rawSchema, err = json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal schema: %w", err)
		}
	}

	var schemaDoc map[string]any
	if err := json.Unmarshal(rawSchema, &schemaDoc); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	schemaDoc["additionalProperties"] = false

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	resourceURL := "https://kandev.local/mcp/schemas/" + url.PathEscape(toolName)
	if err := compiler.AddResource(resourceURL, schemaDoc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

func (s *Server) validateToolArguments(toolName string, req mcp.CallToolRequest) (mcp.CallToolRequest, error) {
	arguments, err := cloneJSONValue(req.GetRawArguments())
	if err != nil {
		return req, fmt.Errorf("invalid arguments for %s: arguments are not valid JSON", toolName)
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	arguments, err = normalizeToolArguments(toolName, arguments)
	if err != nil {
		return req, err
	}
	req.Params.Arguments = arguments

	s.validatorMu.RLock()
	validator, ok := s.toolValidators[toolName]
	s.validatorMu.RUnlock()
	if !ok {
		return req, fmt.Errorf("invalid arguments for %s: schema validator is unavailable", toolName)
	}
	if validator.err != nil || validator.schema == nil {
		return req, fmt.Errorf("invalid arguments for %s: registered schema is invalid", toolName)
	}
	if err := validator.schema.Validate(arguments); err != nil {
		return req, fmt.Errorf("invalid arguments for %s: %w", toolName, err)
	}
	return req, nil
}

func normalizeToolArguments(toolName string, arguments any) (any, error) {
	if toolName != "create_task_kandev" {
		return arguments, nil
	}
	args, ok := arguments.(map[string]any)
	if !ok {
		return arguments, nil
	}
	prompt, hasPrompt := args["prompt"]
	_, hasDescription := args["description"]
	if hasPrompt && hasDescription {
		return nil, fmt.Errorf("invalid arguments for %s: provide either description or prompt, not both", toolName)
	}
	if hasPrompt {
		args["description"] = prompt
		delete(args, "prompt")
	}
	return args, nil
}

func cloneJSONValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
