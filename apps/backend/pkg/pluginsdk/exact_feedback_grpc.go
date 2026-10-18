package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

func (s *grpcHostServer) ListChangeRequestFeedbackExact(ctx context.Context, req *pluginv1.ListChangeRequestFeedbackExactRequest) (*pluginv1.ListChangeRequestFeedbackExactResponse, error) {
	manager, err := s.exactQueryManager()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListChangeRequestFeedback(ctx, ExactChangeRequestFeedbackQuery{RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskIDs: req.GetTaskIds(), Page: exactReadPageFromProto(req.GetPage())})
	if err != nil {
		return nil, err
	}
	items := make([]*pluginv1.ChangeRequestFeedbackTask, len(page.Items))
	for i, item := range page.Items {
		items[i] = feedbackTaskToProto(item)
	}
	return &pluginv1.ListChangeRequestFeedbackExactResponse{Items: items, PageInfo: exactReadPageInfoToProto(page.PageInfo)}, nil
}
func (h grpcExactQueryManager) ListChangeRequestFeedback(ctx context.Context, q ExactChangeRequestFeedbackQuery) (ExactChangeRequestFeedbackPage, error) {
	response, err := h.client.ListChangeRequestFeedbackExact(ctx, &pluginv1.ListChangeRequestFeedbackExactRequest{RequestId: q.RequestID, WorkspaceId: q.WorkspaceID, TaskIds: q.TaskIDs, Page: exactReadPageToProto(q.Page)})
	if err != nil {
		return ExactChangeRequestFeedbackPage{}, err
	}
	items := make([]ChangeRequestFeedbackTask, len(response.GetItems()))
	for i, item := range response.GetItems() {
		items[i] = feedbackTaskFromProto(item)
	}
	return ExactChangeRequestFeedbackPage{Items: items, PageInfo: exactReadPageInfoFromProto(response.GetPageInfo())}, nil
}
func feedbackTaskToProto(row ChangeRequestFeedbackTask) *pluginv1.ChangeRequestFeedbackTask {
	out := &pluginv1.ChangeRequestFeedbackTask{
		TaskId:    row.TaskID,
		Providers: make([]*pluginv1.ChangeRequestFeedbackProviderResult, len(row.Providers)),
		Items:     make([]*pluginv1.ChangeRequestFeedbackItem, len(row.Items)),
	}
	for index, p := range row.Providers {
		out.Providers[index] = &pluginv1.ChangeRequestFeedbackProviderResult{Provider: p.Provider, ProviderState: p.ProviderState, ProviderError: p.ProviderError, ItemsOmitted: p.ItemsOmitted}
	}
	for index, i := range row.Items {
		out.Items[index] = &pluginv1.ChangeRequestFeedbackItem{
			Provider: i.Provider, RepositoryId: i.RepositoryID, RepositoryOwner: i.RepositoryOwner, RepositoryName: i.RepositoryName, Number: i.Number, ItemId: i.ItemID, Kind: i.Kind,
			AuthorLogin: i.AuthorLogin, AuthorKind: i.AuthorKind, ReviewState: i.ReviewState, Resolved: i.Resolved, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, Body: i.Body, BodyTruncated: i.BodyTruncated,
		}
	}
	return out
}
func feedbackTaskFromProto(row *pluginv1.ChangeRequestFeedbackTask) ChangeRequestFeedbackTask {
	out := ChangeRequestFeedbackTask{
		TaskID:    row.GetTaskId(),
		Providers: make([]ChangeRequestFeedbackProviderResult, len(row.GetProviders())),
		Items:     make([]ChangeRequestFeedbackItem, len(row.GetItems())),
	}
	for index, p := range row.GetProviders() {
		out.Providers[index] = ChangeRequestFeedbackProviderResult{Provider: p.GetProvider(), ProviderState: p.GetProviderState(), ProviderError: p.ProviderError, ItemsOmitted: p.GetItemsOmitted()}
	}
	for index, i := range row.GetItems() {
		out.Items[index] = ChangeRequestFeedbackItem{
			Provider: i.GetProvider(), RepositoryID: i.GetRepositoryId(), RepositoryOwner: i.GetRepositoryOwner(), RepositoryName: i.GetRepositoryName(), Number: i.GetNumber(), ItemID: i.GetItemId(), Kind: i.GetKind(),
			AuthorLogin: i.GetAuthorLogin(), AuthorKind: i.GetAuthorKind(), ReviewState: i.GetReviewState(), Resolved: i.Resolved, CreatedAt: i.GetCreatedAt(), UpdatedAt: i.GetUpdatedAt(), Body: i.GetBody(), BodyTruncated: i.GetBodyTruncated(),
		}
	}
	return out
}
