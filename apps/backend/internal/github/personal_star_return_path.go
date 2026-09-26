package github

import (
	"fmt"
	"net/url"
	"strings"
)

// validatePersonalStarReturnPath restricts redirects to same-origin Settings > Plugins paths.
func validatePersonalStarReturnPath(returnPath string) (string, error) {
	trimmed := strings.TrimSpace(returnPath)
	if err := validatePersonalStarReturnPathSyntax(trimmed); err != nil {
		return "", err
	}
	target, err := url.Parse(trimmed)
	if err != nil || target.IsAbs() || target.Host != "" || target.Opaque != "" {
		return "", fmt.Errorf("%w: return path must be relative to this origin", ErrPersonalStarReturnPath)
	}
	if err := validatePersonalStarReturnPathScope(target.Path); err != nil {
		return "", err
	}
	return trimmed, nil
}

func validatePersonalStarReturnPathSyntax(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("%w: return path is required", ErrPersonalStarReturnPath)
	case !strings.HasPrefix(path, "/"):
		return fmt.Errorf("%w: return path must be absolute", ErrPersonalStarReturnPath)
	case strings.HasPrefix(path, "//"):
		return fmt.Errorf("%w: return path must not be protocol-relative", ErrPersonalStarReturnPath)
	case strings.ContainsAny(path, "\\\r\n\t#"):
		return fmt.Errorf("%w: return path contains reserved characters", ErrPersonalStarReturnPath)
	case strings.Contains(path, ":"):
		return fmt.Errorf("%w: return path must not carry a scheme", ErrPersonalStarReturnPath)
	case strings.Contains(path, ".."):
		return fmt.Errorf("%w: return path must not traverse", ErrPersonalStarReturnPath)
	default:
		return nil
	}
}

func validatePersonalStarReturnPathScope(path string) error {
	if path != personalStarReturnPrefix && !strings.HasPrefix(path, personalStarReturnPrefix+"/") {
		return fmt.Errorf("%w: return path must stay under settings plugins", ErrPersonalStarReturnPath)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." || strings.ContainsAny(segment, "%\\") {
			return fmt.Errorf("%w: return path must not contain encoded traversal", ErrPersonalStarReturnPath)
		}
	}
	return nil
}
