package gitlab

import "context"

// GetMRFeedbackForWorkspace resolves only the approved workspace's connection
// and rejects an association whose host no longer matches that connection.
func (s *Service) GetMRFeedbackForWorkspace(ctx context.Context, workspaceID, expectedHost, projectPath string, iid int) (*MRFeedback, error) {
	if expectedHost == "" {
		return nil, ErrTaskMRHostMismatch
	}
	var feedback *MRFeedback
	err := s.RunWithWorkspaceClient(ctx, workspaceID, expectedHost, func(client Client) error {
		var err error
		feedback, err = client.GetMRFeedback(ctx, projectPath, iid)
		return err
	})
	return feedback, err
}
