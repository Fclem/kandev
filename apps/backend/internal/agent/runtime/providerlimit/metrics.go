package providerlimit

import (
	"expvar"
	"strings"

	"go.uber.org/zap"
)

const (
	// MetricContextKanban identifies task-level provider-limit metrics.
	MetricContextKanban = "kanban"
	// MetricContextOffice identifies Office provider-limit metrics.
	MetricContextOffice = "office"

	// MetricFallbackSwitched records a successful transition to a fallback model.
	MetricFallbackSwitched = "switched"
	// MetricFallbackNotAdvertised records a fallback model excluded by capabilities.
	MetricFallbackNotAdvertised = "not_advertised"
	// MetricFallbackMarked records a fallback model already under a provider limit.
	MetricFallbackMarked = "marked"
	// MetricFallbackFailed records a fallback transition that could not be completed.
	MetricFallbackFailed = "failed"

	// MetricWaitArmed records a durable provider-limit wait.
	MetricWaitArmed = "armed"
	// MetricWaitResumed records a successful wait completion.
	MetricWaitResumed = "resumed"
	// MetricWaitCancelled records a durable wait cancellation.
	MetricWaitCancelled = "cancelled"
	// MetricWaitExhausted records a rejected wait after the retry budget is spent.
	MetricWaitExhausted = "exhausted"
	// MetricWaitProbeFailed records a failed provider-limit probe.
	MetricWaitProbeFailed = "probe_failed"
)

var (
	providerLimitMarksTotal    = expvar.NewMap("provider_limit_marks_total")
	providerLimitFallbackTotal = expvar.NewMap("provider_limit_fallback_total")
	providerLimitWaitsTotal    = expvar.NewMap("provider_limit_waits_total")
)

// ObserveMark records an accepted provider-limit mark.
func ObserveMark(logger *zap.Logger, scope, code string) {
	if (scope != ScopeAccount && scope != ScopeModel) || (code != "quota_limited" && code != "rate_limited") {
		return
	}
	providerLimitMarksTotal.Add(metricLabel("scope", scope, "code", code), 1)
	providerLimitLog(logger, "provider_limit.mark", zap.String("scope", scope), zap.String("code", code))
}

// ObserveFallback records one provider-limit fallback decision.
func ObserveFallback(logger *zap.Logger, context, outcome string) {
	if !validMetricContext(context) || !validFallbackOutcome(outcome) {
		return
	}
	providerLimitFallbackTotal.Add(metricLabel("context", context, "outcome", outcome), 1)
	providerLimitLog(logger, "provider_limit.fallback", zap.String("context", context), zap.String("outcome", outcome))
}

// ObserveWait records one provider-limit wait transition.
func ObserveWait(logger *zap.Logger, context, outcome string) {
	if !validMetricContext(context) || !validWaitOutcome(outcome) {
		return
	}
	providerLimitWaitsTotal.Add(metricLabel("context", context, "outcome", outcome), 1)
	providerLimitLog(logger, "provider_limit.wait", zap.String("context", context), zap.String("outcome", outcome))
}

func validMetricContext(value string) bool {
	return value == MetricContextKanban || value == MetricContextOffice
}

func validFallbackOutcome(value string) bool {
	switch value {
	case MetricFallbackSwitched, MetricFallbackNotAdvertised, MetricFallbackMarked, MetricFallbackFailed:
		return true
	default:
		return false
	}
}

func validWaitOutcome(value string) bool {
	switch value {
	case MetricWaitArmed, MetricWaitResumed, MetricWaitCancelled, MetricWaitExhausted, MetricWaitProbeFailed:
		return true
	default:
		return false
	}
}

func metricLabel(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func providerLimitLog(logger *zap.Logger, event string, fields ...zap.Field) {
	if logger != nil {
		logger.Info(event, fields...)
	}
}
