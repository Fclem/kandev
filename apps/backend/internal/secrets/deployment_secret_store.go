package secrets

import (
	"errors"
	"strconv"
	"strings"
)

const deploymentSecretReferencePrefix = "kandev-runtime:github-personal-star:client-secret:"

func ValidateDeploymentSecretReference(reference string) error {
	version, ok := strings.CutPrefix(strings.TrimSpace(reference), deploymentSecretReferencePrefix)
	if !ok || version == "" {
		return errors.New("deployment secret reference is outside the protected namespace")
	}
	if _, err := strconv.ParseInt(version, 10, 64); err != nil {
		return errors.New("deployment secret reference has an invalid version")
	}
	return nil
}
