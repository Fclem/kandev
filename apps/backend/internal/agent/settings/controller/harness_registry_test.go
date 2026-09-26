package controller

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type harnessRegistryTransport func(*http.Request) (*http.Response, error)

func (f harnessRegistryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// AC-AGENTS-RUNTIME-UPDATES-003.11: this request never invokes npm.
func TestHarnessStableLatestUsesTrustedHTTPSRegistryWithoutNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	client := &http.Client{Transport: harnessRegistryTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://registry.npmjs.org/@oh-my-pi%2Fpi-coding-agent" {
			t.Errorf("registry URL = %s", req.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"dist-tags":{"latest":"1.2.3"}}`)), Header: make(http.Header)}, nil
	})}
	updater := &hostRuntimeUpdater{httpClient: client}
	got, err := updater.ResolveHarnessLatest(context.Background(), "@oh-my-pi/pi-coding-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Errorf("stable latest = %q", got)
	}
}
