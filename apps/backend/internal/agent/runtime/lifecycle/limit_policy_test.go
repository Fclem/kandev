package lifecycle

import "testing"

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.3
func TestLimitPolicyFor(t *testing.T) {
	tests := []struct {
		name     string
		profile  *AgentProfileInfo
		fallback string
		resume   bool
	}{
		{name: "nil"},
		{name: "not opted", profile: &AgentProfileInfo{FallbackModel: "fallback"}},
		{name: "eligible", profile: &AgentProfileInfo{FallbackModel: " fallback ", LimitFallback: true, ResumeAfterReset: true}, fallback: "fallback", resume: true},
		{name: "exact preserves reset", profile: &AgentProfileInfo{FallbackModel: "fallback", LimitFallback: true, RequireExactModel: true, ResumeAfterReset: true}, resume: true},
		{name: "automatic preserves reset", profile: &AgentProfileInfo{FallbackModel: "fallback", LimitFallback: true, AutoFallback: true, ResumeAfterReset: true}, resume: true},
		{name: "empty fallback", profile: &AgentProfileInfo{FallbackModel: " ", LimitFallback: true, ResumeAfterReset: true}, resume: true},
		{name: "reset independent", profile: &AgentProfileInfo{ResumeAfterReset: true}, resume: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LimitPolicyFor(tt.profile)
			if got.FallbackModel != tt.fallback || got.ResumeAfterReset != tt.resume {
				t.Fatalf("policy=%+v, want fallback=%q resume=%v", got, tt.fallback, tt.resume)
			}
		})
	}
}
