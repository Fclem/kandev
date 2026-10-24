package github

import "context"

// GetPRFeedbackItemsForWorkspace reads reviews and comments without the UI
// feedback cache or its PR aggregate synchronization side effects. It uses the
// workspace automation credential, so it never depends on a caller identity.
func (s *Service) GetPRFeedbackItemsForWorkspace(
	ctx context.Context, workspaceID, owner, repo string, number int,
) (*PRFeedback, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return nil, err
	}
	if err := requireGitHubCapability(resolved, CapabilityPullRequestRead); err != nil {
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
