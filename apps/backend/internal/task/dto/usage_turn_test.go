package dto

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestToUsageTurnDTOSeparatesDirectChildAndReasoningSubset(t *testing.T) {
	cached := int64(600)
	output := int64(200)
	reasoning := int64(80)
	rows := models.TaskUsageTurnEvents{TurnID: "turn-1", Events: []*models.TaskUsageEvent{
		{
			UsageEventID: "evt-root", NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact",
			ProviderResponseID: "response-root", TokensIn: 400, TokensCachedRead: &cached, TokensOut: &output,
			ReasoningOutputTokens: &reasoning, TokensTotal: 1200, CostSource: "unpriced", OccurredAt: time.Unix(1, 0),
		},
		{
			UsageEventID: "evt-child", NativeScope: "child", MeasurementSource: "response", UsageCompleteness: "exact",
			TokensIn: 10, TokensTotal: 10, TokensOut: &output, CostSubcents: 25, CostSource: "models_dev_list", OccurredAt: time.Unix(2, 0),
		},
	}}
	got := ToUsageTurnDTO(rows, true)
	if got.TurnID != "turn-1" || got.Completeness != "exact" || got.Direct.TotalTokens != 1200 || got.Child.TotalTokens != 10 || got.Total.TotalTokens != 1210 {
		t.Fatalf("usage turn totals = %#v, want direct 1200, child 10, total 1210", got)
	}
	if got.Direct.ThoughtTokens != 0 || got.Responses[0].ReasoningOutputTokens == nil || *got.Responses[0].ReasoningOutputTokens != 80 {
		t.Fatalf("reasoning subset = %#v, want separate detail without changing thought total", got)
	}
	if got.Total.CostSubcents != nil || got.Total.UnpricedCount != 1 {
		t.Fatalf("mixed priced/unpriced total = %#v, want unavailable total price", got.Total)
	}
	if got.LastResponse == nil || got.LastResponse.UsageEventID != "evt-child" {
		t.Fatalf("last response = %#v, want the latest observed event", got.LastResponse)
	}
}

func TestToUsageTurnDTOMarksTokenAggregateOverflow(t *testing.T) {
	got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "turn-overflow", Events: []*models.TaskUsageEvent{
		{UsageEventID: "evt-one", NativeScope: "direct", TokensIn: math.MaxInt64, TokensOut: ptrInt64(0), TokensTotal: math.MaxInt64},
		{UsageEventID: "evt-two", NativeScope: "direct", TokensIn: 1, TokensOut: ptrInt64(0), TokensTotal: 1},
	}}, false)
	if got.Completeness != "partial" || !got.Direct.Overflow || !got.Total.Overflow || got.Direct.InputTokens != math.MaxInt64 {
		t.Fatalf("overflow turn = %#v, want partial saturated totals", got)
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestToUsageTurnDTOReportsZeroCostAndNoResponseRows(t *testing.T) {
	zero := int64(0)
	got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "turn-zero", Events: []*models.TaskUsageEvent{{
		UsageEventID: "evt-zero", NativeScope: "direct", MeasurementSource: "response", UsageCompleteness: "exact",
		TokensOut: &zero, CostSource: "provider_reported", CostSubcents: 0,
	}}}, false)
	if got.Total.CostSubcents == nil || *got.Total.CostSubcents != "0" {
		t.Fatalf("zero cost = %v, want a present zero", got.Total.CostSubcents)
	}
	if len(got.Responses) != 0 || got.LastResponse == nil {
		t.Fatalf("response details = %#v and last response %#v, want summary-only with last response", got.Responses, got.LastResponse)
	}
}

func TestToUsageTurnDTOPreservesCacheMeasurementPresence(t *testing.T) {
	type cacheMeasurement struct {
		Read         int64 `json:"cached_read_tokens"`
		ReadPresent  bool  `json:"cached_read_tokens_present"`
		Write        int64 `json:"cached_write_tokens"`
		WritePresent bool  `json:"cached_write_tokens_present"`
	}
	tests := []struct {
		name   string
		events []*models.TaskUsageEvent
		want   map[string]cacheMeasurement
	}{
		{
			name: "missing read and measured-zero write",
			events: []*models.TaskUsageEvent{
				{TokensCachedWrite: ptrInt64(0), CostSource: "unpriced"},
			},
			want: map[string]cacheMeasurement{
				"direct": {WritePresent: true}, "total": {WritePresent: true}, "child": {},
			},
		},
		{
			name: "measured-zero read and missing write",
			events: []*models.TaskUsageEvent{
				{TokensCachedRead: ptrInt64(0), CostSource: "unpriced"},
			},
			want: map[string]cacheMeasurement{
				"direct": {ReadPresent: true}, "total": {ReadPresent: true}, "child": {},
			},
		},
		{
			name: "mixed direct and child cache measurements",
			events: []*models.TaskUsageEvent{
				{TokensCachedWrite: ptrInt64(2), CostSource: "unpriced"},
				{NativeScope: "child", TokensCachedRead: ptrInt64(3), CostSource: "unpriced"},
			},
			want: map[string]cacheMeasurement{
				"direct": {Write: 2, WritePresent: true},
				"child":  {Read: 3, ReadPresent: true},
				"total":  {Read: 3, ReadPresent: true, Write: 2, WritePresent: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "cache-turn", Events: tt.events}, false)
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var buckets map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &buckets); err != nil {
				t.Fatal(err)
			}
			for name, want := range tt.want {
				var measured cacheMeasurement
				if err := json.Unmarshal(buckets[name], &measured); err != nil {
					t.Fatal(err)
				}
				if measured != want {
					t.Errorf("%s cache measurements = %#v, want %#v", name, measured, want)
				}
			}
		})
	}
}

func TestToUsageTurnDTOAggregatesMeasuredReasoningWithoutResponseDetails(t *testing.T) {
	tests := []struct {
		name   string
		events []*models.TaskUsageEvent
		want   map[string]*int64
	}{
		{
			name: "multiple direct responses, ACP thoughts, and child work",
			events: []*models.TaskUsageEvent{
				{NativeScope: "direct", ReasoningOutputTokens: ptrInt64(10), TokensOut: ptrInt64(200), TokensTotal: 200},
				{NativeScope: "direct", ReasoningOutputTokens: ptrInt64(20), TokensOut: ptrInt64(100), TokensTotal: 100},
				{TokensThought: ptrInt64(7), TokensOut: ptrInt64(10), TokensTotal: 17},
				{NativeScope: "child", ReasoningOutputTokens: ptrInt64(100), TokensOut: ptrInt64(400), TokensTotal: 400},
			},
			want: map[string]*int64{"direct": ptrInt64(37), "child": ptrInt64(100), "total": ptrInt64(137)},
		},
		{
			name: "native reasoning missing despite normalized thought zero",
			events: []*models.TaskUsageEvent{
				{NativeScope: "direct", TokensThought: ptrInt64(0), TokensOut: ptrInt64(10), TokensTotal: 10},
			},
			want: map[string]*int64{"direct": nil, "child": nil, "total": nil},
		},
		{
			name: "native measured zero",
			events: []*models.TaskUsageEvent{
				{NativeScope: "direct", ReasoningOutputTokens: ptrInt64(0), TokensOut: ptrInt64(10), TokensTotal: 10},
			},
			want: map[string]*int64{"direct": ptrInt64(0), "child": nil, "total": ptrInt64(0)},
		},
		{
			name: "ACP measured zero",
			events: []*models.TaskUsageEvent{
				{TokensThought: ptrInt64(0), TokensOut: ptrInt64(10), TokensTotal: 10},
			},
			want: map[string]*int64{"direct": ptrInt64(0), "child": nil, "total": ptrInt64(0)},
		},
		{
			name: "overflow saturates with the existing token aggregates",
			events: []*models.TaskUsageEvent{
				{NativeScope: "direct", ReasoningOutputTokens: ptrInt64(math.MaxInt64), TokensOut: ptrInt64(math.MaxInt64), TokensTotal: math.MaxInt64},
				{NativeScope: "direct", ReasoningOutputTokens: ptrInt64(1), TokensOut: ptrInt64(1), TokensTotal: 1},
			},
			want: map[string]*int64{"direct": ptrInt64(math.MaxInt64), "child": nil, "total": ptrInt64(math.MaxInt64)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: "reasoning-turn", Events: tt.events}, false)
			if len(got.Responses) != 0 {
				t.Fatal("indexed turn unexpectedly included response detail")
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var buckets map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &buckets); err != nil {
				t.Fatal(err)
			}
			for name, want := range tt.want {
				var bucket struct {
					Reasoning *int64 `json:"reasoning_tokens"`
				}
				if err := json.Unmarshal(buckets[name], &bucket); err != nil {
					t.Fatal(err)
				}
				if want == nil {
					if bucket.Reasoning != nil {
						t.Errorf("%s fabricated reasoning measurement %d", name, *bucket.Reasoning)
					}
				} else if bucket.Reasoning == nil || *bucket.Reasoning != *want {
					t.Errorf("%s reasoning = %v, want %d", name, bucket.Reasoning, *want)
				}
			}
		})
	}
}
