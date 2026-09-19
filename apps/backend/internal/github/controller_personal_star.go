package github

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxRepositoryStarStatusBodyBytes = 4 * 1024
	githubAPIErrorCodeField          = "code"
	githubInvalidRequestCode         = "github_invalid_request"
	githubWorkspaceRequiredCode      = "github_workspace_required"
)

type repositoryStarRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	RepositoryURL string `json:"repository_url"`
}

type repositoryStarStatusRequest struct {
	WorkspaceID    string   `json:"workspace_id"`
	RepositoryURLs []string `json:"repository_urls"`
}

func (c *Controller) httpGetPersonalStarConnection(ctx *gin.Context) {
	workspaceID, ok := requiredPersonalStarWorkspace(ctx, ctx.Query("workspace_id"))
	if !ok {
		return
	}
	status, err := c.service.PersonalStarConnectionStatus(ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx))
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpGetRepositoryStarStatus(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxRepositoryStarStatusBodyBytes)
	var request repositoryStarStatusRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeInvalidRepositoryStarRequest(ctx)
		return
	}
	workspaceID, ok := requiredPersonalStarWorkspace(ctx, request.WorkspaceID)
	if !ok {
		return
	}
	states, err := c.service.RepositoryStarStatus(ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx), request.RepositoryURLs)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"states": states})
}

func (c *Controller) httpStarRepository(ctx *gin.Context) {
	c.httpMutateRepositoryStar(ctx, true)
}

func (c *Controller) httpUnstarRepository(ctx *gin.Context) {
	c.httpMutateRepositoryStar(ctx, false)
}

func (c *Controller) httpMutateRepositoryStar(ctx *gin.Context, star bool) {
	var request repositoryStarRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeInvalidRepositoryStarRequest(ctx)
		return
	}
	workspaceID, ok := requiredPersonalStarWorkspace(ctx, request.WorkspaceID)
	if !ok || strings.TrimSpace(request.RepositoryURL) == "" {
		if ok {
			writeInvalidRepositoryStarRequest(ctx)
		}
		return
	}
	var (
		starred bool
		err     error
	)
	if star {
		starred, err = c.service.StarRepository(ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx), request.RepositoryURL)
	} else {
		starred, err = c.service.UnstarRepository(ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx), request.RepositoryURL)
	}
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"starred": starred})
}

func requiredPersonalStarWorkspace(ctx *gin.Context, workspaceID string) (string, bool) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{githubAPIErrorCodeField: githubWorkspaceRequiredCode, commitStatusError: ErrGitHubWorkspaceRequired.Error()})
		return "", false
	}
	return workspaceID, true
}

func writeInvalidRepositoryStarRequest(ctx *gin.Context) {
	ctx.JSON(http.StatusBadRequest, gin.H{githubAPIErrorCodeField: githubInvalidRequestCode, commitStatusError: errMsgInvalidPayload})
}
