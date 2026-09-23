package github

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalidGitHubRepositoryURL = errors.New("invalid GitHub repository URL")
	githubRepositoryOwnerPattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	githubRepositoryNamePattern   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9_-])?$`)
)

// GitHubRepositoryRef is a canonical public GitHub repository identity.
type GitHubRepositoryRef struct {
	Owner string
	Name  string
}

func (r GitHubRepositoryRef) String() string {
	return r.Owner + "/" + r.Name
}

// ParseGitHubRepositoryURL validates a public github.com repository URL and
// returns its canonical owner/name identity.
func ParseGitHubRepositoryURL(raw string) (GitHubRepositoryRef, error) {
	if raw == "" || strings.Contains(raw, "%") {
		return GitHubRepositoryRef{}, ErrInvalidGitHubRepositoryURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || !validGitHubRepositoryURL(parsed) {
		return GitHubRepositoryRef{}, ErrInvalidGitHubRepositoryURL
	}
	owner, name, ok := githubRepositoryURLSegments(parsed)
	if !ok || !githubRepositoryOwnerPattern.MatchString(owner) {
		return GitHubRepositoryRef{}, ErrInvalidGitHubRepositoryURL
	}
	name, ok = canonicalGitHubRepositoryName(name)
	if !ok {
		return GitHubRepositoryRef{}, ErrInvalidGitHubRepositoryURL
	}
	return GitHubRepositoryRef{Owner: strings.ToLower(owner), Name: strings.ToLower(name)}, nil
}

func validGitHubRepositoryURL(parsed *url.URL) bool {
	return parsed.Scheme == httpsScheme && parsed.User == nil && parsed.Port() == "" &&
		strings.EqualFold(parsed.Host, "github.com") && parsed.RawQuery == "" && parsed.Fragment == ""
}

func githubRepositoryURLSegments(parsed *url.URL) (string, string, bool) {
	segments := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", "", false
	}
	return segments[0], segments[1], true
}

func canonicalGitHubRepositoryName(name string) (string, bool) {
	if strings.HasSuffix(strings.ToLower(name), ".git") {
		name = name[:len(name)-len(".git")]
	}
	if strings.HasSuffix(strings.ToLower(name), ".git") || !githubRepositoryNamePattern.MatchString(name) {
		return "", false
	}
	return name, true
}
