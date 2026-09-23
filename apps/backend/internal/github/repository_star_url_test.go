package github

import "testing"

func TestParseGitHubRepositoryURLCanonicalizesValidURLs(t *testing.T) {
	tests := map[string]string{
		"https://GitHub.com/Owner/Repo":        "owner/repo",
		"https://github.com/o/r.git":           "o/r",
		"https://github.com/A-1/R_e.p-o":       "a-1/r_e.p-o",
		"https://github.com/a/repository-name": "a/repository-name",
		// GitHub allows `.`, `-`, and `_` anywhere in a repository name, so a
		// trailing hyphen is a real repository and must keep its star control.
		"https://github.com/acme/widget-": "acme/widget-",
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := ParseGitHubRepositoryURL(raw)
			if err != nil {
				t.Fatalf("ParseGitHubRepositoryURL(%q) error = %v", raw, err)
			}
			if got.String() != want {
				t.Errorf("ParseGitHubRepositoryURL(%q) = %q, want %q", raw, got.String(), want)
			}
		})
	}
}

func TestParseGitHubRepositoryURLRejectsAmbiguousURLs(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/owner/repo",
		"https://github.com:443/owner/repo",
		"https://user@github.com/owner/repo",
		"https://github.com/owner/repo/extra",
		"https://github.com/owner%2Frepo",
		"https://github.com/owner/repo?ref=main",
		"https://github.com/owner/repo#readme",
		"https://github.com/owner/repo.git.git",
		"https://github.com/-owner/repo",
		"https://github.com/owner/repo.",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseGitHubRepositoryURL(raw); err == nil {
				t.Errorf("ParseGitHubRepositoryURL(%q) succeeded", raw)
			}
		})
	}
}
