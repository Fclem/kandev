package github

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

const personalStarOutcomeQueryKey = "star_connection_outcome"

type personalStarConsentStartRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ReturnPath  string `json:"return_path"`
}

func (c *Controller) httpStartPersonalStarConsent(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxRepositoryStarRequestBodyBytes)
	var request personalStarConsentStartRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeInvalidRepositoryStarRequest(ctx)
		return
	}
	sessionID, authorized := personalStarBrowserSession(ctx)
	if !authorized {
		return
	}
	result, err := c.service.StartPersonalStarConsent(
		ctx.Request.Context(), request.WorkspaceID, currentGitHubUserID(ctx), sessionID, request.ReturnPath)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"authorization_url": result.AuthorizationURL,
		"state_expires_at":  result.StateExpiresAt,
	})
}

func (c *Controller) httpCompletePersonalStarConsent(ctx *gin.Context) {
	query, err := ParseGitHubStarCallbackRawQuery(ctx.Request.URL.RawQuery)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	identity, _ := authn.IdentityFromContext(ctx.Request.Context())
	outcome, err := c.service.CompletePersonalStarConsent(
		ctx.Request.Context(), query, strings.TrimSpace(identity.UserID), strings.TrimSpace(identity.SessionID))
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Redirect(http.StatusFound, personalStarOutcomeRedirect(outcome))
}

func (c *Controller) httpDisconnectPersonalStarConnection(ctx *gin.Context) {
	workspaceID, ok := requiredPersonalStarWorkspace(ctx, ctx.Query("workspace_id"))
	if !ok {
		return
	}
	if err := c.service.DisconnectPersonalStarConnection(ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx)); err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// personalStarBrowserSession requires a real browser session when
// authentication is enabled: token-only and synthetic identities cannot start a
// user-visible consent.
func personalStarBrowserSession(ctx *gin.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx.Request.Context())
	if !ok || identity.Synthetic {
		return "", true
	}
	sessionID := strings.TrimSpace(identity.SessionID)
	if sessionID == "" {
		writeGitHubAuthError(ctx, ErrGitHubPersonalRequired)
		return "", false
	}
	return sessionID, true
}

// personalStarOutcomeRedirect preserves the validated return path's own query
// parameters while carrying exactly one outcome value.
func personalStarOutcomeRedirect(outcome PersonalStarCallbackOutcome) string {
	target, err := url.Parse(outcome.ReturnPath)
	if err != nil {
		return personalStarReturnPrefix
	}
	query := target.Query()
	query.Del(personalStarOutcomeQueryKey)
	query.Set(personalStarOutcomeQueryKey, outcome.Outcome)
	target.RawQuery = query.Encode()
	return target.String()
}
