package costs

// CostListResponse wraps a list of cost events.
type CostListResponse struct {
	Costs []*CostEvent `json:"costs"`
}

// CostBreakdownResponse wraps aggregated cost data.
type CostBreakdownResponse struct {
	Breakdown []*CostBreakdown `json:"breakdown"`
}

// CostBreakdownComposedResponse contains the summary and four grouped projections.
type CostBreakdownComposedResponse struct {
	TotalSubcents    *int64           `json:"total_subcents"`
	CoverageDegraded bool             `json:"coverage_degraded"`
	ByAgent          []*CostBreakdown `json:"by_agent"`
	ByProject        []*CostBreakdown `json:"by_project"`
	ByModel          []*CostBreakdown `json:"by_model"`
	ByProvider       []*CostBreakdown `json:"by_provider"`
}

// CreateBudgetRequest is the request body for creating a budget policy.
// LimitSubcents is hundredths of a cent (see BudgetPolicy doc).
type CreateBudgetRequest struct {
	ScopeType         string `json:"scope_type"`
	ScopeID           string `json:"scope_id"`
	LimitSubcents     int64  `json:"limit_subcents"`
	Period            string `json:"period"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	ActionOnExceed    string `json:"action_on_exceed"`
}

// UpdateBudgetRequest is the request body for updating a budget policy.
type UpdateBudgetRequest struct {
	ScopeType         *string `json:"scope_type,omitempty"`
	ScopeID           *string `json:"scope_id,omitempty"`
	LimitSubcents     *int64  `json:"limit_subcents,omitempty"`
	Period            *string `json:"period,omitempty"`
	AlertThresholdPct *int    `json:"alert_threshold_pct,omitempty"`
	ActionOnExceed    *string `json:"action_on_exceed,omitempty"`
}

// BudgetListResponse wraps a list of budget policies.
type BudgetListResponse struct {
	Budgets []*BudgetPolicy `json:"budgets"`
}

// DefaultCeilingResponse reports the built-in default ceiling's current
// effective limit (AC-OFFICE-BUDGET-003.5).
type DefaultCeilingResponse struct {
	LimitSubcents int64 `json:"limit_subcents"`
}

// SetDefaultCeilingRequest is the request body for writing the built-in
// default ceiling's limit.
type SetDefaultCeilingRequest struct {
	LimitSubcents int64 `json:"limit_subcents"`
}
