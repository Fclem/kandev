package pluginsdk

type ExactChangeRequestFeedbackQuery struct {
	RequestID   string
	WorkspaceID string
	TaskIDs     []string
	Page        ExactReadPage
}

type ChangeRequestFeedbackProviderResult struct {
	Provider      string
	ProviderState string
	ProviderError *string
	ItemsOmitted  int64
}

type ChangeRequestFeedbackItem struct {
	Provider        string
	RepositoryID    string
	RepositoryOwner string
	RepositoryName  string
	Number          int64
	ItemID          string
	Kind            string
	AuthorLogin     string
	AuthorKind      string
	ReviewState     string
	Resolved        *bool
	CreatedAt       string
	UpdatedAt       string
	Body            string
	BodyTruncated   bool
}

type ChangeRequestFeedbackTask struct {
	TaskID    string
	Providers []ChangeRequestFeedbackProviderResult
	Items     []ChangeRequestFeedbackItem
}

type ExactChangeRequestFeedbackPage struct {
	Items    []ChangeRequestFeedbackTask
	PageInfo ExactReadPageInfo
}
