package github

import "context"

// GetPRFeedbackItemsForWorkspace reads reviews and comments without the UI
// feedback cache or its PR aggregate synchronization side effects.
func (s *Service) GetPRFeedbackItemsForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string, number int,
) (*PRFeedback, error) {
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return nil, err
	}
	reviews, err := resolved.Client.ListPRReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	comments, err := resolved.Client.ListPRComments(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, err
	}
	return &PRFeedback{Reviews: reviews, Comments: comments}, nil
}
