package secrets

import (
	"errors"
	"strconv"
	"strings"
)

const deploymentSecretReferencePrefix = "kandev-runtime:github-personal-star:client-secret:"

func ValidateDeploymentSecretReference(reference string) error {
	version, ok := strings.CutPrefix(reference, deploymentSecretReferencePrefix)
	if !ok || version == "" {
		return errors.New("deployment secret reference is outside the protected namespace")
	}
	parsed, err := strconv.ParseInt(version, 10, 64)
	if err != nil || parsed < 1 || strconv.FormatInt(parsed, 10) != version {
		return errors.New("deployment secret reference has an invalid version")
	}
	return nil
}
