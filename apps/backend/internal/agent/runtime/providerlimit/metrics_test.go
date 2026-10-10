package providerlimit

import (
	"context"
	"expvar"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestProviderLimitMetricAcceptedMarksOnly(t *testing.T) {
	svc, persist, now := newLimitFixture()
	core, observed := observer.New(zapcore.InfoLevel)
	svc.logger = zap.New(core)
	profile := &settingsmodels.AgentProfile{ID: "metric-profile", AgentID: "saved-claude", BillingType: "api_key"}
	subject := svc.ForProfile(profile, nil)
	for _, tc := range []struct {
		scope string
		code  routingerr.Code
		label string
	}{
		{scope: "account", code: routingerr.CodeQuotaLimited, label: "scope=account;code=quota_limited"},
		{scope: "model", code: routingerr.CodeRateLimited, label: "scope=model;code=rate_limited"},
	} {
		before := providerLimitMetricValue(t, "provider_limit_marks_total", tc.label)
		if _, err := svc.Record(context.Background(), subject, "opus", &routingerr.Error{Code: tc.code, LimitScope: tc.scope}, *now); err != nil {
			t.Fatalf("record accepted mark: %v", err)
		}
		if got := providerLimitMetricValue(t, "provider_limit_marks_total", tc.label) - before; got != 1 {
			t.Errorf("mark metric delta for %s = %d, want 1", tc.label, got)
		}
	}
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("accepted mark logs = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		fields := entry.ContextMap()
		if entry.Message != "provider_limit.mark" || len(fields) != 2 {
			t.Errorf("mark event = %q %#v, want only scope/code", entry.Message, fields)
		}
	}

	before := providerLimitMetricValue(t, "provider_limit_marks_total", "scope=model;code=rate_limited")
	persist.fail = true
	if _, err := svc.Record(context.Background(), subject, "sonnet", &routingerr.Error{Code: routingerr.CodeRateLimited}, *now); err == nil {
		t.Fatal("failed mark persistence was accepted")
	}
	if _, limited := svc.Lookup(subject, "opus"); !limited {
		t.Fatal("lookup should retain the existing mark")
	}
	if got := providerLimitMetricValue(t, "provider_limit_marks_total", "scope=model;code=rate_limited") - before; got != 0 {
		t.Fatalf("failed persistence or lookup changed mark metric by %d", got)
	}
}

func providerLimitMetricValue(t *testing.T, name, label string) int64 {
	t.Helper()
	value := expvar.Get(name)
	if value == nil {
		t.Fatalf("expvar counter %q is not registered", name)
	}
	entry := value.(*expvar.Map).Get(label)
	if entry == nil {
		return 0
	}
	counter, ok := entry.(*expvar.Int)
	if !ok {
		return 0
	}
	return counter.Value()
}
func TestProviderLimitMetricLabelSetsAndStructuredLogs(t *testing.T) {
	core, observed := observer.New(zapcore.InfoLevel)
	log := zap.New(core)
	fallbackOutcomes := []string{
		MetricFallbackSwitched, MetricFallbackNotAdvertised, MetricFallbackMarked, MetricFallbackFailed,
	}
	waitOutcomes := []string{
		MetricWaitArmed, MetricWaitResumed, MetricWaitCancelled, MetricWaitExhausted, MetricWaitProbeFailed,
	}
	for _, context := range []string{MetricContextKanban, MetricContextOffice} {
		for _, outcome := range fallbackOutcomes {
			label := metricLabel("context", context, "outcome", outcome)
			before := providerLimitMetricValue(t, "provider_limit_fallback_total", label)
			ObserveFallback(log, context, outcome)
			if delta := providerLimitMetricValue(t, "provider_limit_fallback_total", label) - before; delta != 1 {
				t.Errorf("fallback counter %s delta = %d, want 1", label, delta)
			}
		}
		for _, outcome := range waitOutcomes {
			label := metricLabel("context", context, "outcome", outcome)
			before := providerLimitMetricValue(t, "provider_limit_waits_total", label)
			ObserveWait(log, context, outcome)
			if delta := providerLimitMetricValue(t, "provider_limit_waits_total", label) - before; delta != 1 {
				t.Errorf("wait counter %s delta = %d, want 1", label, delta)
			}
		}
	}

	before := providerLimitMetricValue(t, "provider_limit_fallback_total", "context=kanban;outcome=switched")
	ObserveFallback(log, "profile-id", "task-id")
	if delta := providerLimitMetricValue(t, "provider_limit_fallback_total", "context=kanban;outcome=switched") - before; delta != 0 {
		t.Fatalf("invalid labels changed fallback counter by %d", delta)
	}
	for _, name := range []string{"provider_limit_marks_total", "provider_limit_fallback_total", "provider_limit_waits_total"} {
		value := expvar.Get(name)
		value.(*expvar.Map).Do(func(keyValue expvar.KeyValue) {
			key := keyValue.Key
			if strings.Contains(key, "profile") || strings.Contains(key, "binding") ||
				strings.Contains(key, "task") || strings.Contains(key, "session") || strings.Contains(key, "run") {
				t.Errorf("%s label contains an identifier: %q", name, key)
			}
		})
	}

	entries := observed.All()
	if len(entries) != 18 {
		t.Fatalf("structured metric events = %d, want 18", len(entries))
	}
	for _, entry := range entries {
		if entry.Message != "provider_limit.fallback" && entry.Message != "provider_limit.wait" {
			t.Errorf("unexpected structured event %q", entry.Message)
		}
		fields := entry.ContextMap()
		if len(fields) != 2 {
			t.Errorf("%s fields = %#v, want only fixed labels", entry.Message, fields)
		}
		if _, ok := fields["profile_id"]; ok {
			t.Errorf("%s leaked profile identifier", entry.Message)
		}
		if _, ok := fields["task_id"]; ok {
			t.Errorf("%s leaked task identifier", entry.Message)
		}
	}
}
