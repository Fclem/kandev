---
status: shipped
created: 2026-08-01
owner: kandev
---

# MCP Tool Argument Validation

Decision: [ADR-2026-08-01-validate-mcp-tool-arguments](../../decisions/2026-08-01-validate-mcp-tool-arguments.md)

## Why

Agents and external MCP clients need a failed tool call to be distinguishable from a successful operation. Silently ignoring an incorrect argument can create tasks, start agents, or mutate configuration without the caller's intended data while still reporting success.

## What

- Every Kandev MCP tool validates its arguments against the schema registered for its current MCP mode before running its handler.
- Missing required arguments, values of the wrong declared type, violated declared constraints, and unknown top-level arguments return a tool error and cause no handler or backend side effect.
- Parameterless tools accept omitted arguments or an empty object and reject any supplied field.
- Validation follows mode changes: after the server changes its registered tool set, calls use the replacement tools' schemas.
- Nested configuration objects remain open when their schema intentionally permits arbitrary keys.
- `create_task_kandev` accepts `prompt` as an unadvertised compatibility alias for the advertised `description` field, without adding another field or explanation to the tool schema. A call containing both names fails rather than choosing one silently.
- Existing valid tool calls and registered schemas remain unchanged.

## API surface

The advertised MCP `tools/list` schemas do not gain repeated unknown-argument metadata or the `create_task_kandev.prompt` compatibility alias. The server applies these rules when handling `tools/call`:

1. Normalize an explicitly supported compatibility alias, if any.
2. Validate the resulting arguments against the currently registered tool schema with the root object closed to unknown fields.
3. Invoke the handler only when validation succeeds.

Validation failures are returned as MCP tool error results. They identify the invalid argument location and violated constraint without returning sensitive argument values.

## Failure modes

- If tool arguments fail validation, Kandev returns an error and does not invoke the handler.
- If a registered tool schema cannot compile, calls to that tool fail closed and Kandev logs the schema defect; test coverage prevents shipping an uncompilable built-in schema.
- If both `description` and compatibility alias `prompt` are supplied to `create_task_kandev`, Kandev returns an error and creates no task.
- Validation errors do not echo complete prompts, credentials, configuration values, or other argument contents.

## Scenarios

- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller omits one of its schema-required arguments, **THEN** the call returns a tool error and its handler is not invoked.
- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller supplies a value with the wrong schema type, **THEN** the call returns a tool error and its handler is not invoked.
- **GIVEN** any registered Kandev MCP tool, **WHEN** a caller supplies an unknown top-level argument, **THEN** the call returns a tool error naming its location and its handler is not invoked.
- **GIVEN** a parameterless tool, **WHEN** a caller omits arguments or supplies `{}`, **THEN** the handler runs normally; **WHEN** it supplies any field, **THEN** the call fails.
- **GIVEN** a tool with an intentionally open nested configuration map, **WHEN** a caller supplies an arbitrary key inside that map, **THEN** validation accepts the nested key.
- **GIVEN** the server changes MCP mode, **WHEN** a caller invokes a tool in the replacement set, **THEN** validation uses that tool's replacement schema.
- **GIVEN** a caller supplies only `prompt` to `create_task_kandev`, **WHEN** validation runs, **THEN** the prompt is forwarded unchanged through the existing `description` field.
- **GIVEN** a caller supplies both `prompt` and `description` to `create_task_kandev`, **WHEN** validation runs, **THEN** the call returns an error and creates no task.

## Out of scope

- Adding the compatibility alias to the advertised schema or tool description.
- Changing the semantic business rules enforced inside individual handlers and services.
- Closing arbitrary-key nested configuration maps that are intentionally open.
- Changing MCP transport, authorization, tool availability by mode, or backend action payloads.
