package service

import (
	"encoding/json"
	"errors"
)

var errNegativePromptUsageMeasurement = errors.New("negative prompt usage measurement")

func (d *PromptUsageData) UnmarshalJSON(data []byte) error {
	type payload PromptUsageData
	decoded := struct {
		*payload
		Usage *UsageTokens `json:"usage"`
	}{payload: (*payload)(d)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Usage != nil {
		d.Usage = *decoded.Usage
	} else if isNativeUnavailableResponse(d.UsageObservation) {
		d.UsageUnavailable = true
		d.Usage = UsageTokens{Estimated: true, PriceSuppressed: true}
	}
	return nil
}

func (u *UsageTokens) UnmarshalJSON(data []byte) error {
	type tokens UsageTokens
	decoded := struct {
		*tokens
		InputTokens  *int64 `json:"input_tokens"`
		OutputTokens *int64 `json:"output_tokens"`
	}{tokens: (*tokens)(u)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.InputTokens != nil {
		u.InputTokens = *decoded.InputTokens
		u.InputTokensPresent = true
	}
	if decoded.OutputTokens != nil {
		u.OutputTokens = *decoded.OutputTokens
		if u.OutputTokensPresent == nil {
			present := true
			u.OutputTokensPresent = &present
		}
	} else if u.OutputTokensPresent == nil {
		present := false
		u.OutputTokensPresent = &present
	}
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CachedReadTokens < 0 ||
		u.CachedWriteTokens < 0 || u.ThoughtTokens < 0 || u.TotalTokens < 0 ||
		u.ProviderReportedCostSubcents < 0 {
		return errNegativePromptUsageMeasurement
	}
	return nil
}
