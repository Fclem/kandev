package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

func TestInputPresenceControlsPricingAndProjection(t *testing.T) {
	for _, output := range []int64{0, 20} {
		for _, input := range []string{"absent", "null", "zero", "producer zero", "positive"} {
			t.Run(fmt.Sprintf("%s/output-%d", input, output), func(t *testing.T) {
				counts := map[string]any{"output_tokens": output, "output_tokens_present": true}
				switch input {
				case "null":
					counts["input_tokens"] = nil
				case "zero":
					counts["input_tokens"] = 0
				case "producer zero":
					counts["input_tokens_present"] = true
				case "positive":
					counts["input_tokens"] = 5
				}
				payload, err := decodePayload(&bus.Event{Data: map[string]any{
					"task_id": "task-1", "turn_id": "turn-1", "usage_event_id": "event-1", "model": "model-1", "usage": counts,
				}})
				if err != nil {
					t.Fatal(err)
				}
				pricing := &fakePricingLookup{ok: true, pricing: commoncosts.ModelPricing{InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}}
				row := (&Writer{pricing: pricing}).buildRow(context.Background(), payload, fixedOccurredAt)
				if row == nil || row.TokensOut == nil || *row.TokensOut != output {
					t.Fatalf("known output disappeared: %+v", row)
				}
				known := input == "zero" || input == "producer zero" || input == "positive"
				turn := dto.ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "turn-1", Events: []*models.TaskUsageEvent{row}}, true)
				if (turn.Total.CostSubcents != nil) != known {
					t.Fatalf("calculated cost = %v, want presence %t", turn.Total.CostSubcents, known)
				}
				encoded, err := json.Marshal(turn)
				if err != nil {
					t.Fatal(err)
				}
				var wire struct {
					Total struct {
						InputComplete bool `json:"input_complete"`
					} `json:"total"`
					Responses []struct {
						InputPresent bool `json:"input_tokens_present"`
					} `json:"responses"`
				}
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Total.InputComplete != known || wire.Responses[0].InputPresent != known {
					t.Fatalf("input measurement presence = %t/%t, want %t", wire.Total.InputComplete, wire.Responses[0].InputPresent, known)
				}
				if !known {
					payload.Usage.ProviderReportedCostPresent = true
					row = (&Writer{pricing: pricing}).buildRow(context.Background(), payload, fixedOccurredAt)
					if row.CostSource != CostSourceProviderReported || row.CostSubcents != 0 {
						t.Fatalf("provider-reported zero was suppressed: %+v", row)
					}
				}
			})
		}
	}
}

func TestMissingOutputDoesNotProduceCatalogueEstimate(t *testing.T) {
	for _, measured := range []bool{false, true} {
		counts := map[string]any{"input_tokens": 5, "output_tokens_present": measured}
		if measured {
			counts["output_tokens"] = 0
		}
		payload, err := decodePayload(&bus.Event{Data: map[string]any{
			"task_id": "task-1", "model": "model-1", "usage": counts,
		}})
		if err != nil {
			t.Fatal(err)
		}
		pricing := &fakePricingLookup{ok: true, pricing: commoncosts.ModelPricing{InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}}
		row := (&Writer{pricing: pricing}).buildRow(context.Background(), payload, fixedOccurredAt)
		wantSource := CostSourceUnpriced
		if measured {
			wantSource = CostSourceModelsDevList
		}
		if row == nil || row.CostSource != wantSource || (measured && row.CostSubcents != 5) {
			t.Fatalf("output measured %t produced incorrect cost: %+v", measured, row)
		}
	}
}
