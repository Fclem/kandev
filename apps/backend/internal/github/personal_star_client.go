package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// PersonalStarClient is the narrow capability exposed to the dedicated OAuth
// App token path. General GitHub clients cannot star repositories through it.
type PersonalStarClient interface {
	IsStarred(context.Context, GitHubRepositoryRef) (bool, error)
	Star(context.Context, GitHubRepositoryRef) (bool, error)
	Unstar(context.Context, GitHubRepositoryRef) (bool, error)
}

// PersonalStarHTTPClient implements the GitHub REST starring contract with a
// dedicated personal OAuth App access token.
type PersonalStarHTTPClient struct {
	client *TokenClient
}

func NewPersonalStarClient(token string) *PersonalStarHTTPClient {
	return &PersonalStarHTTPClient{client: NewTokenClient(token, TokenPrincipal{Kind: TokenCredentialPersonalStar})}
}

func (c *PersonalStarHTTPClient) IsStarred(ctx context.Context, repository GitHubRepositoryRef) (bool, error) {
	status, err := c.request(ctx, http.MethodGet, repository)
	if err != nil {
		var apiError *GitHubAPIError
		if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	if status != http.StatusNoContent {
		return false, fmt.Errorf("unexpected GitHub star status response %d", status)
	}
	return true, nil
}

func (c *PersonalStarHTTPClient) Star(ctx context.Context, repository GitHubRepositoryRef) (bool, error) {
	if status, err := c.request(ctx, http.MethodPut, repository); err != nil {
		return false, err
	} else if status != http.StatusNoContent {
		return false, fmt.Errorf("unexpected GitHub star response %d", status)
	}
	return true, nil
}

func (c *PersonalStarHTTPClient) Unstar(ctx context.Context, repository GitHubRepositoryRef) (bool, error) {
	if status, err := c.request(ctx, http.MethodDelete, repository); err != nil {
		return false, err
	} else if status != http.StatusNoContent {
		return false, fmt.Errorf("unexpected GitHub unstar response %d", status)
	}
	return false, nil
}

func (c *PersonalStarHTTPClient) request(ctx context.Context, method string, repository GitHubRepositoryRef) (int, error) {
	if c == nil || c.client == nil || c.client.httpClient == nil {
		return 0, ErrGitHubNotConfigured
	}
	endpoint := "/user/starred/" + repository.String()
	request, err := http.NewRequestWithContext(ctx, method, githubAPIBase+endpoint, nil)
	if err != nil {
		return 0, err
	}
	c.client.setGitHubHeaders(request)
	request.Header.Set("Authorization", "Bearer "+c.client.token)
	response, err := c.client.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("request %s %s: %w", method, endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	c.client.recordRateHeaders(response, endpoint)
	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		c.client.maybeMarkRateExhaustedFromBody(endpoint, response.StatusCode, body)
		return 0, &GitHubAPIError{StatusCode: response.StatusCode, Endpoint: endpoint, Body: string(body)}
	}
	return response.StatusCode, nil
}
