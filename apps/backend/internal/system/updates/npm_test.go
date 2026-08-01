package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLatestNightlyFromResolvesDistTagAndExactVersion(t *testing.T) {
	const version = "1.2.4-nightly.shaabc123def456"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.npm.install-v1+json" {
			t.Errorf("Accept=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dist-tags":{"nightly":"` + version + `"},"versions":{"` + version + `":{"name":"kandev"}}}`))
	}))
	defer server.Close()

	got, packageURL, err := FetchLatestNightlyFrom(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("FetchLatestNightlyFrom: %v", err)
	}
	if got != "v"+version {
		t.Fatalf("version=%q want %q", got, "v"+version)
	}
	if packageURL != "https://www.npmjs.com/package/kandev/v/"+version {
		t.Fatalf("packageURL=%q", packageURL)
	}
}

func TestFetchLatestNightlyFromRejectsInvalidRegistryDocuments(t *testing.T) {
	const valid = "1.2.4-nightly.shaabc123def456"
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing tag", body: `{"dist-tags":{},"versions":{}}`, want: "missing nightly dist-tag"},
		{name: "invalid version", body: `{"dist-tags":{"nightly":"1.2.4-nightly.shaBAD"},"versions":{}}`, want: "invalid nightly version"},
		{name: "missing exact record", body: `{"dist-tags":{"nightly":"` + valid + `"},"versions":{}}`, want: "missing exact version"},
		{name: "malformed json", body: `{`, want: "decode npm response"},
		{
			name: "trailing json value",
			body: `{"dist-tags":{"nightly":"` + valid + `"},"versions":{"` + valid + `":{"name":"kandev"}}} {}`,
			want: "decode npm response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, _, err := FetchLatestNightlyFrom(context.Background(), server.Client(), server.URL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestFetchLatestNightlyFromReturnsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "registry down", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, _, err := FetchLatestNightlyFrom(context.Background(), server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "npm status 503") {
		t.Fatalf("error=%v", err)
	}
}
