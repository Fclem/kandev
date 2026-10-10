package lifecycle

import "context"

// PromptAdmissionHook commits dispatch ownership after generation admission and before provider I/O.
type PromptAdmissionHook func(executionID string, generation uint64) error

type promptAdmissionHookContextKey struct{}

func WithPromptAdmissionHook(ctx context.Context, hook PromptAdmissionHook) context.Context {
	return context.WithValue(ctx, promptAdmissionHookContextKey{}, hook)
}

func promptAdmissionHookFromContext(ctx context.Context) PromptAdmissionHook {
	hook, _ := ctx.Value(promptAdmissionHookContextKey{}).(PromptAdmissionHook)
	return hook
}
