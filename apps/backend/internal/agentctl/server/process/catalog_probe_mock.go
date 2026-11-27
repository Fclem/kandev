package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

const mockCatalogCapabilityID = "mock_acp_catalog_v1"

var errMockCatalogMalformed = errors.New("mock catalog probe response is malformed")

func mockCatalogCapability() catalogProbeCapability {
	return catalogProbeCapability{id: mockCatalogCapabilityID, acquire: acquireMockCatalog}
}

// acquireMockCatalog sends one ACP initialize request carrying the fixture's
// read-only extension. It never creates, loads or prompts a session.
func acquireMockCatalog(ctx context.Context, helper *catalogHelper) (sessioncatalog.Discovery, error) {
	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": acp.AgentMethodInitialize,
		"params": acp.InitializeRequest{
			ProtocolVersion: acp.ProtocolVersionNumber,
			Meta:            map[string]any{sessioncatalog.MockCatalogProbeMetaKey: true},
		},
	})
	if err != nil {
		return failedCatalogDiscovery(), err
	}
	if _, err := helper.stdin.Write(append(request, '\n')); err != nil {
		return failedCatalogDiscovery(), err
	}
	type readResult struct {
		discovery sessioncatalog.Discovery
		err       error
	}
	done := make(chan readResult, 1)
	go func() {
		discovery, err := readMockCatalogResponse(helper)
		done <- readResult{discovery: discovery, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = helper.stdout.Close()
		<-done
		return failedCatalogDiscovery(), ctx.Err()
	case result := <-done:
		return result.discovery, result.err
	}
}

func readMockCatalogResponse(helper *catalogHelper) (sessioncatalog.Discovery, error) {
	scanner := bufio.NewScanner(helper.stdout)
	scanner.Buffer(make([]byte, 64<<10), sessioncatalog.MaxTaskProbePayloadBytes)
	for scanner.Scan() {
		var message struct {
			ID     json.RawMessage `json:"id"`
			Error  json.RawMessage `json:"error"`
			Result struct {
				Meta map[string]json.RawMessage `json:"_meta"`
			} `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil || string(message.ID) != "1" {
			continue
		}
		raw, ok := message.Result.Meta[sessioncatalog.MockCatalogProbeMetaKey]
		if len(message.Error) > 0 || !ok {
			return failedCatalogDiscovery(), errMockCatalogMalformed
		}
		var discovery sessioncatalog.Discovery
		if err := json.Unmarshal(raw, &discovery); err != nil {
			return failedCatalogDiscovery(), errMockCatalogMalformed
		}
		return discovery, nil
	}
	return failedCatalogDiscovery(), errMockCatalogMalformed
}
