package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultNPMRegistryURL = "https://registry.npmjs.org/kandev"

type npmPackagePayload struct {
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
}

func FetchLatestNightlyFrom(ctx context.Context, client *http.Client, registryURL string) (string, string, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}
	if registryURL == "" {
		registryURL = DefaultNPMRegistryURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("build npm request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	req.Header.Set("User-Agent", "kandev-updates-poller")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("npm request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("npm status %d: %s", resp.StatusCode, string(body))
	}

	payload, err := decodeNPMPackagePayload(resp.Body)
	if err != nil {
		return "", "", err
	}
	version := payload.DistTags[string(ChannelNightly)]
	if version == "" {
		return "", "", errors.New("npm response missing nightly dist-tag")
	}
	// npm package versions and dist-tag targets use canonical bare SemVer.
	// Internal update tags add v only after this registry integrity check.
	if !isCanonicalNPMNightlyVersion(version) {
		return "", "", fmt.Errorf("npm response has invalid nightly version %q", version)
	}
	record, ok := payload.Versions[version]
	if !ok || len(record) == 0 || string(record) == "null" {
		return "", "", fmt.Errorf("npm response missing exact version %q", version)
	}
	var object map[string]interface{}
	if err := json.Unmarshal(record, &object); err != nil || object == nil {
		return "", "", fmt.Errorf("npm response has invalid exact version record %q", version)
	}

	packageVersion := strings.TrimPrefix(version, "v")
	return "v" + packageVersion, "https://www.npmjs.com/package/kandev/v/" + url.PathEscape(packageVersion), nil
}

func isCanonicalNPMNightlyVersion(version string) bool {
	return !strings.HasPrefix(version, "v") && isNightlyVersion(version)
}

func decodeNPMPackagePayload(reader io.Reader) (npmPackagePayload, error) {
	decoder := json.NewDecoder(reader)
	var payload npmPackagePayload
	if err := decoder.Decode(&payload); err != nil {
		return npmPackagePayload{}, fmt.Errorf("decode npm response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return npmPackagePayload{}, fmt.Errorf("decode npm response: %w", err)
	}
	return payload, nil
}
