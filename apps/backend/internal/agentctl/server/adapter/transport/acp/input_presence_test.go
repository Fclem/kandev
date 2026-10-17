package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
)

func TestPromptUsageInputPresence(t *testing.T) {
	for _, shape := range []string{"typed", "metadata", "gemini"} {
		for _, input := range []string{"absent", "null", "zero", "positive"} {
			t.Run(shape+"/"+input, func(t *testing.T) {
				inputKey, outputKey, totalKey := "input_tokens", "output_tokens", "total_tokens"
				if shape == "typed" {
					inputKey, outputKey, totalKey = "inputTokens", "outputTokens", "totalTokens"
				}
				counts := map[string]any{outputKey: 20, totalKey: 20}
				switch input {
				case "null":
					counts[inputKey] = nil
				case "zero":
					counts[inputKey] = 0
				case "positive":
					counts[inputKey], counts[totalKey] = 5, 25
				}
				result := map[string]any{"stopReason": "end_turn"}
				switch shape {
				case "typed":
					result["usage"] = counts
				case "metadata":
					result["_meta"] = map[string]any{"usage": counts}
				case "gemini":
					result["_meta"] = map[string]any{"quota": map[string]any{"token_count": counts}}
				}
				response := promptUsageWireResponse(t, result)
				usage := extractUsage(&response)
				if usage == nil || usage.OutputTokens != 20 {
					t.Fatalf("known output disappeared: %+v", usage)
				}
				encoded, err := json.Marshal(usage)
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]any
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatal(err)
				}
				wantPresent := input == "zero" || input == "positive"
				if wire["input_tokens_present"] != wantPresent {
					t.Fatalf("input presence = %v, want %t (wire %s)", wire["input_tokens_present"], wantPresent, encoded)
				}
				if !wantPresent {
					if _, ok := wire["input_tokens"]; ok {
						t.Fatalf("missing input became a measurement: %s", encoded)
					}
				}
			})
		}
	}
}

func TestTypedPromptUsageMissingOutputIsNotMeasuredZero(t *testing.T) {
	response := promptUsageWireResponse(t, map[string]any{
		"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 5, "totalTokens": 5},
	})
	usage := extractUsage(&response)
	if usage == nil || usage.InputTokens != 5 || usage.OutputTokensPresent {
		t.Fatalf("missing output became a measurement or erased known input: %+v", usage)
	}
}
func TestPromptUsageZeroMeasurements(t *testing.T) {
	tests := []struct {
		name     string
		counts   map[string]any
		input    bool
		output   bool
		optional string
	}{
		{name: "absent", counts: map[string]any{}},
		{name: "null", counts: map[string]any{"input_tokens": nil, "output_tokens": nil, "total_tokens": nil}},
		{name: "output zero", counts: map[string]any{"output_tokens": 0, "total_tokens": 0}, output: true},
		{name: "null input output zero", counts: map[string]any{"input_tokens": nil, "output_tokens": 0}, output: true},
		{name: "input zero", counts: map[string]any{"input_tokens": 0}, input: true},
		{name: "both zero", counts: map[string]any{"input_tokens": 0, "output_tokens": 0}, input: true, output: true},
		{name: "cache read zero", counts: map[string]any{"cached_read_tokens": 0}, optional: "cached_read_tokens_present"},
		{name: "cache write zero", counts: map[string]any{"cached_write_tokens": 0}, optional: "cached_write_tokens_present"},
		{name: "thought zero", counts: map[string]any{"thought_tokens": 0}, optional: "thought_tokens_present"},
	}
	for _, shape := range []string{"typed", "metadata", "gemini"} {
		for _, tc := range tests {
			if shape == "gemini" && tc.optional != "" {
				continue
			}
			t.Run(shape+"/"+tc.name, func(t *testing.T) {
				response := promptUsageWireResponse(t, zeroPromptUsageResult(shape, tc.counts))
				usage := extractUsage(&response)
				if !tc.input && !tc.output && tc.optional == "" {
					if usage != nil {
						t.Fatalf("absent-only usage became measured: %+v", usage)
					}
					return
				}
				if usage == nil {
					t.Fatal("explicitly measured zero usage disappeared")
				}
				encoded, err := json.Marshal(usage)
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]any
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatal(err)
				}
				if wire["input_tokens_present"] != tc.input || wire["output_tokens_present"] != tc.output {
					t.Fatalf("required zero presence changed: %s", encoded)
				}
				if tc.optional != "" && wire[tc.optional] != true {
					t.Fatalf("optional measured zero disappeared: %s", encoded)
				}
				if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 {
					t.Fatalf("zero observation became a nonzero count: %s", encoded)
				}
			})
		}
	}
}

func TestPromptUsageRejectsMalformedCounts(t *testing.T) {
	for _, shape := range []string{"metadata", "gemini"} {
		for name, invalid := range map[string]float64{
			"fraction": 0.5, "negative": -1, "out of range": math.Exp2(63),
		} {
			for _, valid := range []string{"none", "input", "output zero"} {
				t.Run(shape+"/"+name+"/"+valid, func(t *testing.T) {
					counts := map[string]any{"input_tokens": invalid, "output_tokens": invalid}
					switch valid {
					case "input":
						counts["input_tokens"] = 5
					case "output zero":
						counts["output_tokens"] = 0
					}
					response := promptUsageWireResponse(t, zeroPromptUsageResult(shape, counts))
					usage := extractUsage(&response)
					if valid == "none" {
						if usage != nil {
							t.Fatalf("all-invalid counts became measured: %+v", usage)
						}
						return
					}
					if usage == nil || usage.InputTokensPresent != (valid == "input") ||
						usage.OutputTokensPresent != (valid == "output zero") {
						t.Fatalf("invalid count became present or erased valid sibling: %+v", usage)
					}
					encoded, err := json.Marshal(usage)
					if err != nil {
						t.Fatal(err)
					}
					var wire map[string]any
					if err := json.Unmarshal(encoded, &wire); err != nil {
						t.Fatal(err)
					}
					if wire["input_tokens_present"] != (valid == "input") ||
						wire["output_tokens_present"] != (valid == "output zero") {
						t.Fatalf("wire presence admitted an invalid count: %s", encoded)
					}
				})
			}
		}
	}
	t.Run("invalid optional categories", func(t *testing.T) {
		response := promptUsageWireResponse(t, zeroPromptUsageResult("metadata", map[string]any{
			"input_tokens": 5, "output_tokens": 20, "cached_read_tokens": 0.5,
			"cached_write_tokens": -1, "thought_tokens": math.Exp2(63), "total_tokens": 0.5,
		}))
		usage := extractUsage(&response)
		if usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 20 ||
			usage.CachedReadTokensPresent || usage.CachedWriteTokensPresent ||
			usage.ThoughtTokensPresent || usage.TotalTokens != 0 {
			t.Fatalf("invalid optional count became measured: %+v", usage)
		}
	})
}

func TestTypedPromptUsageRejectsNegativeCounts(t *testing.T) {
	for _, category := range []string{
		"input_tokens", "output_tokens", "cached_read_tokens", "cached_write_tokens", "thought_tokens", "total_tokens",
	} {
		t.Run(category, func(t *testing.T) {
			counts := map[string]any{"input_tokens": 5, "output_tokens": 20, category: -1}
			response := promptUsageWireResponse(t, zeroPromptUsageResult("typed", counts))
			usage := extractUsage(&response)
			if usage == nil || usage.InputTokens < 0 || usage.OutputTokens < 0 ||
				usage.CachedReadTokens < 0 || usage.CachedWriteTokens < 0 ||
				usage.ThoughtTokens < 0 || usage.TotalTokens < 0 {
				t.Fatalf("negative category published: %+v", usage)
			}
			if usage.InputTokensPresent != (category != "input_tokens") ||
				usage.OutputTokensPresent != (category != "output_tokens") ||
				usage.CachedReadTokensPresent || usage.CachedWriteTokensPresent || usage.ThoughtTokensPresent {
				t.Fatalf("invalid category became measured or erased valid sibling: %+v", usage)
			}
		})
	}
	response := promptUsageWireResponse(t, zeroPromptUsageResult("typed", map[string]any{
		"input_tokens": -1, "output_tokens": -1, "total_tokens": -1,
		"cached_read_tokens": -1, "cached_write_tokens": -1, "thought_tokens": -1,
	}))
	if usage := extractUsage(&response); usage != nil {
		t.Fatalf("all-negative usage admitted: %+v", usage)
	}
}

func TestPromptUsageRejectsAmbiguousWireCounts(t *testing.T) {
	for _, shape := range []string{"metadata", "gemini"} {
		for _, number := range []json.Number{
			"9007199254740992", "9007199254740993", "9223372036854775807",
		} {
			for _, known := range []string{"none", "input zero", "output zero"} {
				t.Run(shape+"/"+string(number)+"/"+known, func(t *testing.T) {
					counts := map[string]any{"input_tokens": number, "output_tokens": number}
					switch known {
					case "input zero":
						counts["input_tokens"] = 0
					case "output zero":
						counts["output_tokens"] = 0
					}
					response := promptUsageWireResponse(t, zeroPromptUsageResult(shape, counts))
					usage := extractUsage(&response)
					if known == "none" {
						if usage != nil {
							t.Fatalf("ambiguous wire counts became measured: %+v", usage)
						}
						return
					}
					if usage == nil || usage.InputTokensPresent != (known == "input zero") ||
						usage.OutputTokensPresent != (known == "output zero") ||
						usage.InputTokens != 0 || usage.OutputTokens != 0 {
						t.Fatalf("ambiguous count became measured or erased known zero: %+v", usage)
					}
				})
			}
		}
	}
}

func TestPromptUsagePreservesExactWireBoundaries(t *testing.T) {
	for _, tc := range []struct {
		shape string
		count json.Number
		want  int64
	}{
		{shape: "metadata", count: "9007199254740991", want: 9007199254740991},
		{shape: "gemini", count: "9007199254740991", want: 9007199254740991},
		{shape: "typed", count: "9223372036854775807", want: math.MaxInt64},
	} {
		t.Run(tc.shape, func(t *testing.T) {
			response := promptUsageWireResponse(t, zeroPromptUsageResult(tc.shape, map[string]any{
				"input_tokens": tc.count, "output_tokens": 0,
			}))
			usage := extractUsage(&response)
			if usage == nil || !usage.InputTokensPresent || !usage.OutputTokensPresent || usage.InputTokens != tc.want {
				t.Fatalf("exact wire boundary changed: %+v, want %d", usage, tc.want)
			}
		})
	}
}

func TestTokenCountNumericBoundaries(t *testing.T) {
	for _, value := range []any{
		math.NaN(), math.Inf(1), math.Inf(-1), -0.5, int(-1), int64(-1), math.Exp2(63),
	} {
		if count, present := getInt64WithPresence(map[string]any{"tokens": value}, "tokens"); present || count != 0 {
			t.Fatalf("invalid numeric count %v admitted as %d", value, count)
		}
	}
	for _, value := range []any{float64(0), int(0), int64(0), int64(math.MaxInt64)} {
		if _, present := getInt64WithPresence(map[string]any{"tokens": value}, "tokens"); !present {
			t.Fatalf("valid integral count %v rejected", value)
		}
	}
}

func zeroPromptUsageResult(shape string, counts map[string]any) map[string]any {
	result := map[string]any{"stopReason": "end_turn"}
	switch shape {
	case "typed":
		keys := map[string]string{
			"input_tokens": "inputTokens", "output_tokens": "outputTokens", "total_tokens": "totalTokens",
			"cached_read_tokens": "cachedReadTokens", "cached_write_tokens": "cachedWriteTokens", "thought_tokens": "thoughtTokens",
		}
		typed := make(map[string]any, len(counts))
		for key, value := range counts {
			typed[keys[key]] = value
		}
		result["usage"] = typed
	case "metadata":
		result["_meta"] = map[string]any{"usage": counts}
	case "gemini":
		result["_meta"] = map[string]any{"quota": map[string]any{"token_count": counts}}
	}
	return result
}

func TestPromptUsageEscapedPropertyPresence(t *testing.T) {
	for _, resultKey := range []string{`"result"`, `"res\u0075lt"`} {
		for _, input := range []string{"", `"inputTokens":null,`} {
			t.Run(resultKey+"/"+input, func(t *testing.T) {
				raw := []byte(`{"stopReason":"end_turn","us\u0061ge":{` + input + `"outputTokens":20,"totalTokens":20}}`)
				response := promptUsageRawWireResponse(t, raw, resultKey)
				usage := extractUsage(&response)
				if usage == nil || usage.InputTokensPresent || !usage.OutputTokensPresent || usage.OutputTokens != 20 {
					t.Fatalf("escaped key fabricated input or erased output: %+v", usage)
				}
			})
		}
	}
}

func TestGrokReasoningRawWirePrecision(t *testing.T) {
	for _, value := range []json.Number{"9007199254740991", "9007199254740992", "9007199254740993", "0"} {
		t.Run(string(value), func(t *testing.T) {
			response := promptUsageWireResponse(t, map[string]any{
				"stopReason": "end_turn", "_meta": map[string]any{"usage": map[string]any{
					"input_tokens": 0, "output_tokens": 0, "reasoningTokens": value,
				}},
			})
			usage := normalizeGrokPromptUsage(extractUsage(&response), response.Meta)
			wantPresent := value == "9007199254740991" || value == "0"
			if usage == nil || usage.ThoughtTokensPresent != wantPresent || !usage.InputTokensPresent || !usage.OutputTokensPresent {
				t.Fatalf("rounded reasoning admitted or valid zero erased: %+v", usage)
			}
			if value == "9007199254740991" && usage.ThoughtTokens != 9007199254740991 {
				t.Fatalf("safe reasoning changed: %+v", usage)
			}
		})
	}
}

func TestGrokReasoningOnlyRawWireUsage(t *testing.T) {
	for _, value := range []any{json.Number("20"), json.Number("0"), nil, json.Number("-1"), json.Number("0.5"), "20"} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			response := promptUsageWireResponse(t, map[string]any{
				"stopReason": "end_turn", "_meta": map[string]any{"usage": map[string]any{"reasoningTokens": value}},
			})
			usage := normalizeGrokPromptUsage(extractUsage(&response), response.Meta)
			want := value == json.Number("20") || value == json.Number("0")
			if !want {
				if usage != nil {
					t.Fatalf("invalid-only usage admitted: %+v", usage)
				}
				return
			}
			if usage == nil || !usage.ThoughtTokensPresent || usage.InputTokensPresent || usage.OutputTokensPresent {
				t.Fatalf("known reasoning erased or required counts fabricated: %+v", usage)
			}
		})
	}
}

func promptUsageWireResponse(t *testing.T, result map[string]any) sdk.PromptResponse {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return promptUsageRawWireResponse(t, raw, `"result"`)
}

func promptUsageRawWireResponse(t *testing.T, result []byte, resultKey string) sdk.PromptResponse {
	t.Helper()
	toAgent, fromClient := io.Pipe()
	toClient, fromAgent := io.Pipe()
	t.Cleanup(func() {
		_ = toAgent.Close()
		_ = fromClient.Close()
		_ = toClient.Close()
		_ = fromAgent.Close()
	})
	conn := acpclient.NewClientSideConnectionWithLogger(acpclient.NewClient(), fromClient, toClient, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	responses := make(chan sdk.PromptResponse, 1)
	errors := make(chan error, 1)
	go func() {
		defer func() { _ = fromClient.Close() }()
		response, err := conn.Prompt(ctx, sdk.PromptRequest{SessionId: "session-1"})
		responses <- response
		errors <- err
	}()
	line, err := bufio.NewReader(toAgent).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(line, &request); err != nil {
		t.Fatal(err)
	}
	id, err := json.Marshal(request["id"])
	if err != nil {
		t.Fatal(err)
	}
	frame := append([]byte(`{"jsonrpc":"2.0","id":`+string(id)+`,`+resultKey+`:`), result...)
	frame = append(frame, '}', '\n')
	if _, err := fromAgent.Write(frame); err != nil {
		t.Fatal(err)
	}
	response := <-responses
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	return response
}
