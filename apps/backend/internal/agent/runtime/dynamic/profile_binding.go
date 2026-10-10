package dynamic

import (
	"sort"
	"strings"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// ProfileCredentialBindingDescriptor contains credential locators, never values.
func ProfileCredentialBindingDescriptor(profile *agentsettingsmodels.AgentProfile, billingType string) CredentialBindingDescriptor {
	if profile == nil {
		return CredentialBindingDescriptor{}
	}
	descriptor := CredentialBindingDescriptor{
		Version:              1,
		AgentFamilyID:        profile.AgentID,
		AuthenticationMethod: strings.TrimSpace(billingType),
		ExecutorNamespace:    "local",
		AuthorizationScope:   "agent_runtime",
	}
	secretIDs := make([]string, 0, len(profile.EnvVars))
	for _, envVar := range profile.EnvVars {
		if strings.TrimSpace(envVar.SecretID) != "" {
			secretIDs = append(secretIDs, strings.TrimSpace(envVar.SecretID))
		}
	}
	if len(secretIDs) > 0 {
		sort.Strings(secretIDs)
		descriptor.CredentialSourceKind = "profile_secret"
		descriptor.CredentialLocator = strings.Join(secretIDs, ",")
	} else if descriptor.AuthenticationMethod != "" {
		descriptor.CredentialSourceKind = "agent_credentials"
		descriptor.CredentialLocator = profile.AgentID + ":" + descriptor.AuthenticationMethod
	}
	return descriptor
}
