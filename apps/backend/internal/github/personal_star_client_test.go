package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersonalStarClientUsesGitHubStarringEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		invoke     func(PersonalStarClient) (bool, error)
		want       bool
	}{
		{
			name: "status starred", statusCode: http.StatusNoContent,
			invoke: func(client PersonalStarClient) (bool, error) {
				return client.IsStarred(context.Background(), GitHubRepositoryRef{Owner: "owner", Name: "repo"})
			}, want: true,
		},
		{
			name: "status unstarred", statusCode: http.StatusNotFound,
			invoke: func(client PersonalStarClient) (bool, error) {
				return client.IsStarred(context.Background(), GitHubRepositoryRef{Owner: "owner", Name: "repo"})
			}, want: false,
		},
		{
			name: "star", statusCode: http.StatusNoContent,
			invoke: func(client PersonalStarClient) (bool, error) {
				return client.Star(context.Background(), GitHubRepositoryRef{Owner: "owner", Name: "repo"})
			}, want: true,
		},
		{
			name: "unstar", statusCode: http.StatusNoContent,
			invoke: func(client PersonalStarClient) (bool, error) {
				return client.Unstar(context.Background(), GitHubRepositoryRef{Owner: "owner", Name: "repo"})
			}, want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/user/starred/owner/repo" {
					t.Errorf("path = %q", request.URL.Path)
				}
				if request.Header.Get("Authorization") != "Bearer token" {
					t.Errorf("authorization = %q", request.Header.Get("Authorization"))
				}
				body, _ := io.ReadAll(request.Body)
				if len(body) != 0 {
					t.Errorf("body = %q, want empty", body)
				}
				w.WriteHeader(tc.statusCode)
			}))
			t.Cleanup(server.Close)

			client := NewPersonalStarClient("token")
			client.client.httpClient = &http.Client{Transport: &rewriteTransport{base: server.URL}}
			got, err := tc.invoke(client)
			if err != nil {
				t.Fatalf("request error = %v", err)
			}
			if got != tc.want {
				t.Errorf("result = %v, want %v", got, tc.want)
			}
		})
	}
}
