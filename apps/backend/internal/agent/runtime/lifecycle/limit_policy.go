package lifecycle

import "strings"

// LimitPolicy is the effective recovery policy, independent of saved dormant settings.
type LimitPolicy struct {
	FallbackModel    string
	ResumeAfterReset bool
}

// LimitPolicyFor applies concrete profile policy without changing saved choices.
func LimitPolicyFor(profile *AgentProfileInfo) LimitPolicy {
	if profile == nil {
		return LimitPolicy{}
	}
	policy := LimitPolicy{ResumeAfterReset: profile.ResumeAfterReset}
	if profile.LimitFallback && !profile.RequireExactModel && !profile.AutoFallback {
		policy.FallbackModel = strings.TrimSpace(profile.FallbackModel)
	}
	return policy
}
