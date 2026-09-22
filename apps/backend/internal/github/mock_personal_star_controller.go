package github

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	mockPersonalStarWorkspaceRequired = "workspace_id is required"
	mockE2ESelectorValue              = "true"
)

// Star control routes are mounted only for E2E runs: registration requires both
// the E2E profile selector and a MockClient, so a production-profile process
// with KANDEV_MOCK_GITHUB=true still exposes no controls.
type mockPersonalStarStateRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	UserID        string `json:"user_id"`
	RepositoryURL string `json:"repository_url"`
	Starred       bool   `json:"starred"`
}

type mockPersonalStarFailureRequest struct {
	Status   bool `json:"status"`
	Mutation bool `json:"mutation"`
}

type mockPersonalStarConnectionRequest struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Login       string `json:"login"`
	PublicRepo  bool   `json:"public_repo"`
	Outcome     string `json:"outcome"`
}

type mockPersonalStarOutcomeRequest struct {
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Outcome     string `json:"outcome"`
}

func (c *MockController) registerPersonalStarRoutes(api *gin.RouterGroup) {
	api.PUT("/personal-star-state", c.setPersonalStarState)
	api.PUT("/personal-star-failures", c.setPersonalStarFailures)
	api.PUT("/personal-star-connection", c.setPersonalStarConnection)
	api.DELETE("/personal-star-connection", c.deletePersonalStarConnection)
	api.PUT("/personal-star-outcome", c.setPersonalStarOutcome)
}

func (c *MockController) setPersonalStarState(ctx *gin.Context) {
	var request mockPersonalStarStateRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: "invalid personal star state request"})
		return
	}
	repository, err := ParseGitHubRepositoryURL(request.RepositoryURL)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: err.Error()})
		return
	}
	c.mock.SetPersonalStarState(request.WorkspaceID, request.UserID, repository.String(), request.Starred)
	ctx.Status(http.StatusNoContent)
}

func (c *MockController) setPersonalStarFailures(ctx *gin.Context) {
	var request mockPersonalStarFailureRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: "invalid personal star failure request"})
		return
	}
	c.mock.SetPersonalStarFailures(request.Status, request.Mutation)
	ctx.Status(http.StatusNoContent)
}

func (c *MockController) setPersonalStarConnection(ctx *gin.Context) {
	var request mockPersonalStarConnectionRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: "invalid personal star connection request"})
		return
	}
	outcome := strings.TrimSpace(request.Outcome)
	if outcome == "" {
		outcome = personalStarOutcomeConnected
	}
	if err := c.service.SeedPersonalStarConnectionForE2E(
		ctx.Request.Context(), request.WorkspaceID, request.UserID, request.Login, request.PublicRepo, outcome,
	); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{personalStarParamError: err.Error()})
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *MockController) deletePersonalStarConnection(ctx *gin.Context) {
	workspaceID := strings.TrimSpace(ctx.Query("workspace_id"))
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: mockPersonalStarWorkspaceRequired})
		return
	}
	if err := c.service.ClearPersonalStarConnectionForE2E(
		ctx.Request.Context(), workspaceID, strings.TrimSpace(ctx.Query("user_id")),
	); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{personalStarParamError: err.Error()})
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *MockController) setPersonalStarOutcome(ctx *gin.Context) {
	var request mockPersonalStarOutcomeRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.WorkspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{personalStarParamError: "invalid personal star outcome request"})
		return
	}
	if err := c.service.SetPersonalStarReconnectOutcomeForE2E(
		ctx.Request.Context(), request.WorkspaceID, request.UserID, strings.TrimSpace(request.Outcome),
	); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{personalStarParamError: err.Error()})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// personalStarE2EControlsEnabled reports whether the star controls may be
// mounted. Both conditions are required on purpose.
func personalStarE2EControlsEnabled(mock *MockClient, getenv func(string) string) bool {
	return mock != nil && getenv != nil && getenv("KANDEV_E2E_MOCK") == mockE2ESelectorValue
}
