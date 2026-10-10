package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) recordManualProviderLimitNotice(ctx context.Context, taskID, sessionID string, origin launchOrigin, model string) error {
	if s.providerLimits == nil || s.messageCreator == nil || origin != launchOriginManual && origin != "" {
		return nil
	}
	office, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil || office {
		return err
	}
	mark, model, limited, err := s.manualProviderLimitMark(ctx, taskID, sessionID, model)
	if err != nil || !limited {
		return err
	}
	metadata := map[string]interface{}{
		detailsKeyKind: "provider_limit_notice", metaKeyVariant: metaVariantWarning, metaKeyModelID: model, "reset_known": mark.ResetKnown,
	}
	if mark.ResetKnown {
		metadata[metaKeyRetryAt] = mark.Until.UTC().Format(time.RFC3339Nano)
	}
	identity := "provider-limit-notice:" + sessionID + ":" + mark.Key + ":" + mark.Until.UTC().Format(time.RFC3339Nano)
	messageID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(identity)).String()
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.messageCreator.CreateSessionMessageIdempotent(ctx, messageID, taskID, "The selected model is limited. Sending anyway.", sessionID, string(v1.MessageTypeStatus), turnID, metadata, false)
}

// manualProviderLimitMark returns the active mark for the model a manual
// prompt will use, defaulting to the session's selected model.
func (s *Service) manualProviderLimitMark(ctx context.Context, taskID, sessionID, model string) (providerlimit.Mark, string, bool, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return providerlimit.Mark{}, "", false, err
	}
	if session == nil || session.TaskID != taskID {
		return providerlimit.Mark{}, "", false, fmt.Errorf("provider limit notice session does not belong to the task")
	}
	subject, selectedModel, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic {
		return providerlimit.Mark{}, "", false, err
	}
	if model == "" {
		model = selectedModel
	}
	mark, limited := s.providerLimits.Lookup(subject, model)
	return mark, model, limited, nil
}
