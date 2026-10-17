package models

import "time"

// TaskUsageTotals is the read-side aggregate over task_usage_events for one
// scope (a task, or a single session within a task) - docs/specs/task-cost-ledger/spec.md
// AC-18, AC-19, AC-20. TokensTotal is the sum of each row's stored
// tokens_total column, never recomputed from the per-kind sums at read time.
// Required-token completeness distinguishes stored lower bounds from exact
// counts. Empty scopes have complete zero sums and nil timestamps.
type TaskUsageTotals struct {
	TokensIn             int64
	TokensCachedRead     int64
	TokensCachedWrite    int64
	TokensOut            int64
	TokensThought        int64
	TokensTotal          int64
	CostSubcents         int64
	EventCount           int64
	EstimatedEventCount  int64
	UnpricedEventCount   int64
	InputTokensComplete  bool
	TotalTokensComplete  bool
	OutputTokensComplete bool
	FirstEventAt         *time.Time
	LastEventAt          *time.Time
}
