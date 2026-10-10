package runtime

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// LimitPolicy is the effective provider-limit recovery policy for a profile.
type LimitPolicy = lifecycle.LimitPolicy

// StartModelPolicy constrains the model a launch may select.
type StartModelPolicy = lifecycle.StartModelPolicy

// CachedModelState is the last model state reported by a live execution.
type CachedModelState = lifecycle.CachedModelState

// PromptAdmissionHook commits dispatch ownership after generation admission and
// before provider I/O.
type PromptAdmissionHook = lifecycle.PromptAdmissionHook

// ErrAgentReported marks a failure reported by the agent itself.
var ErrAgentReported = lifecycle.ErrAgentReported

const (
	ModelSelectionReasonRequestedNotAdvertised = lifecycle.ModelSelectionReasonRequestedNotAdvertised
	ModelSelectionReasonFallbackNotAdvertised  = lifecycle.ModelSelectionReasonFallbackNotAdvertised
	ModelSelectionReasonCatalogEmpty           = lifecycle.ModelSelectionReasonCatalogEmpty
)

// LimitPolicyForProfile applies a concrete profile's saved recovery settings.
func LimitPolicyForProfile(profile *settingsmodels.AgentProfile) LimitPolicy {
	if profile == nil {
		return LimitPolicy{}
	}
	return lifecycle.LimitPolicyFor(&lifecycle.AgentProfileInfo{
		FallbackModel:     profile.FallbackModel,
		LimitFallback:     profile.LimitFallback,
		RequireExactModel: profile.RequireExactModel,
		AutoFallback:      profile.AutoFallback,
		ResumeAfterReset:  profile.ResumeAfterReset,
	})
}

// WithStartModelPolicy attaches a launch model constraint to ctx.
func WithStartModelPolicy(ctx context.Context, policy StartModelPolicy) context.Context {
	return lifecycle.WithStartModelPolicy(ctx, policy)
}

// WithPromptAdmissionHook attaches a dispatch-ownership hook to ctx.
func WithPromptAdmissionHook(ctx context.Context, hook PromptAdmissionHook) context.Context {
	return lifecycle.WithPromptAdmissionHook(ctx, hook)
}

// IsModelUnavailableBootstrapFailure reports whether err is a bootstrap failure
// caused by the requested or fallback model not being advertised.
func IsModelUnavailableBootstrapFailure(err error) bool {
	var failure *BootstrapFailure
	if !errors.As(err, &failure) {
		return false
	}
	switch failure.Reason {
	case ModelSelectionReasonRequestedNotAdvertised,
		ModelSelectionReasonFallbackNotAdvertised,
		ModelSelectionReasonCatalogEmpty:
		return true
	default:
		return false
	}
}
