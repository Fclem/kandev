package sessioncatalog

import (
	"errors"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

// ValidateDiscovery accepts terminal evidence without inferring support from a
// nonempty list. Failed and unsupported reports cannot advertise fresh choices.
func ValidateDiscovery(result *Discovery) error {
	if result == nil {
		return errors.New("missing terminal discovery")
	}
	if err := validateDiscoveryOutcome(result); err != nil {
		return err
	}
	if err := validateDiscoverySurface(result.Models); err != nil {
		return err
	}
	return validateDiscoverySurface(result.Modes)
}

func validateDiscoveryOutcome(result *Discovery) error {
	switch result.Outcome {
	case OutcomeComplete:
		if (!result.Models.Present || !result.Models.Supported) && (!result.Modes.Present || !result.Modes.Supported) {
			return errors.New("complete discovery has no observed supported surface")
		}
	case OutcomeUnsupported, OutcomeFailed:
		if result.Models.Supported || result.Modes.Supported || len(result.Models.Choices) > 0 || len(result.Modes.Choices) > 0 || result.Models.Catalog != nil || result.Modes.Catalog != nil {
			return errors.New("unsuccessful discovery cannot advertise choices")
		}
	default:
		return errors.New("unknown terminal discovery outcome")
	}
	return nil
}

func validateDiscoverySurface(surface Surface) error {
	if len(surface.Choices) > modelcatalog.MaxNodes {
		return errors.New("discovery exceeds choice limit")
	}
	if (!surface.Present || !surface.Supported) && (len(surface.Choices) > 0 || surface.Catalog != nil) {
		return errors.New("choices require an observed supported surface")
	}
	for _, choice := range surface.Choices {
		if choice.ID == "" || len(choice.ID) > modelcatalog.MaxIDLength || len(choice.Name) > modelcatalog.MaxStringLength || len(choice.Description) > modelcatalog.MaxStringLength {
			return errors.New("invalid discovery choice")
		}
	}
	if surface.Catalog != nil {
		return modelcatalog.Validate(surface.Catalog)
	}
	return nil
}
