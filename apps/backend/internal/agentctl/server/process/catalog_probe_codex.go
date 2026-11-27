package process

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

// codexProbeClientName matches the live adapter's client declaration.
const codexProbeClientName = "kandev"

// codexQualification is one exact @openai/codex version whose app-server
// model/list contract passed same-contract native qualification. There are no
// version ranges: a new reviewed pin stays notice-only until it is requalified
// and listed here.
type codexQualification struct {
	capabilityID string
	packageSpec  string
	// userAgentPrefix is the actual process implementation evidence: the
	// provider derives it from the client name and its own version.
	userAgentPrefix string
}

var codexQualifications = []codexQualification{
	{
		capabilityID:    "codex_app_server_model_list_v1@0.169.0",
		packageSpec:     "@openai/codex@0.169.0",
		userAgentPrefix: codexProbeClientName + "/0.169.0 ",
	},
	{
		capabilityID:    "codex_app_server_model_list_v1@0.171.0",
		packageSpec:     "@openai/codex@0.171.0",
		userAgentPrefix: codexProbeClientName + "/0.171.0 ",
	},
}

func codexQualificationFor(packageSpec string) (codexQualification, bool) {
	for _, qualification := range codexQualifications {
		if qualification.packageSpec == packageSpec {
			return qualification, true
		}
	}
	return codexQualification{}, false
}

var (
	errCatalogProbeServerRequest = errors.New("catalog probe does not serve provider requests")
	errCodexModelListMalformed   = errors.New("codex model/list page is malformed")
	errCodexModelListBound       = errors.New("codex model/list exceeded its bounds")
	errCodexModelListCursor      = errors.New("codex model/list repeated a cursor")
)

func codexModelListCapability(qualification codexQualification) catalogProbeCapability {
	return catalogProbeCapability{
		id: qualification.capabilityID,
		acquire: func(ctx context.Context, helper *catalogHelper) (sessioncatalog.Discovery, error) {
			return acquireCodexModelCatalog(ctx, helper, qualification.userAgentPrefix)
		},
	}
}

// acquireCodexModelCatalog sends only initialize, initialized and complete
// paginated model/list. No thread, turn, configuration or model setter is
// ever sent, so the helper cannot start or alter a conversation.
func acquireCodexModelCatalog(ctx context.Context, helper *catalogHelper, userAgentPrefix string) (sessioncatalog.Discovery, error) {
	client := protocol.NewClient(helper.stdin, helper.stdout, protocol.Options{
		MaxFrameBytes: sessioncatalog.MaxTaskProbePayloadBytes + 64<<10,
	})
	defer func() { _ = client.Close() }()
	client.SetRequestHandler(func(context.Context, protocol.ServerRequest) (any, error) {
		return nil, errCatalogProbeServerRequest
	})
	title := "Kandev"
	var initialized protocol.InitializeResponse
	if err := client.Call(ctx, protocol.MethodInitialize, protocol.InitializeParams{
		ClientInfo:   protocol.ClientInfo{Name: codexProbeClientName, Title: &title, Version: "1.0.0"},
		Capabilities: protocol.InitializeCapabilities{ExperimentalAPI: true},
	}, &initialized); err != nil {
		return failedCatalogDiscovery(), err
	}
	if !strings.HasPrefix(initialized.UserAgent, userAgentPrefix) {
		return unsupportedCatalogDiscovery(), nil
	}
	if err := client.Notify(ctx, protocol.MethodInitialized, struct{}{}); err != nil {
		return failedCatalogDiscovery(), err
	}
	choices, err := readCodexModelPages(ctx, client)
	if err != nil {
		return failedCatalogDiscovery(), err
	}
	return sessioncatalog.Discovery{
		Outcome: sessioncatalog.OutcomeComplete,
		Models:  sessioncatalog.Surface{Present: true, Supported: true, Choices: choices},
	}, nil
}

func readCodexModelPages(ctx context.Context, client *protocol.Client) ([]sessioncatalog.Choice, error) {
	choices := make([]sessioncatalog.Choice, 0)
	seenCursors := make(map[string]struct{})
	seenModels := make(map[string]struct{})
	var cursor *string
	payload := 0
	for range sessioncatalog.MaxTaskProbePages {
		raw, err := client.CallRaw(ctx, protocol.MethodModelList, protocol.ModelListParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		payload += len(raw)
		if payload > sessioncatalog.MaxTaskProbePayloadBytes {
			return nil, errCodexModelListBound
		}
		var page struct {
			Data       *[]protocol.Model `json:"data"`
			NextCursor *string           `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil || page.Data == nil {
			return nil, errCodexModelListMalformed
		}
		if choices, err = appendCodexModelChoices(choices, *page.Data, seenModels); err != nil {
			return nil, err
		}
		if page.NextCursor == nil {
			return choices, nil
		}
		next := *page.NextCursor
		if next == "" {
			return nil, errCodexModelListMalformed
		}
		if _, repeated := seenCursors[next]; repeated {
			return nil, errCodexModelListCursor
		}
		seenCursors[next] = struct{}{}
		cursor = &next
	}
	return nil, errCodexModelListBound
}

func appendCodexModelChoices(choices []sessioncatalog.Choice, models []protocol.Model, seen map[string]struct{}) ([]sessioncatalog.Choice, error) {
	for _, model := range models {
		id := model.Model
		if id == "" {
			id = model.ID
		}
		if strings.TrimSpace(id) == "" {
			return nil, errCodexModelListMalformed
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errCodexModelListMalformed
		}
		seen[id] = struct{}{}
		name := model.DisplayName
		if name == "" {
			name = id
		}
		choices = append(choices, sessioncatalog.Choice{ID: id, Name: name, Description: model.Description})
		if len(choices) > sessioncatalog.MaxTaskProbeChoices {
			return nil, errCodexModelListBound
		}
	}
	return choices, nil
}

func failedCatalogDiscovery() sessioncatalog.Discovery {
	return sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}
}

func unsupportedCatalogDiscovery() sessioncatalog.Discovery {
	return sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeUnsupported}
}
