package manifest

import "fmt"

// MinimumMessagesCapabilityVersion is the first Host release that supports
// both Go Messages().List and the browser conversation facade safely.
const MinimumMessagesCapabilityVersion = "0.91.1"

// MinimumSessionActivityVersion is the first Host release that supports
// session activity and turn reads.
const MinimumSessionActivityVersion = "0.97.1"

// RequiresMessagesCapabilityMinimum reports whether a manifest opts into the
// messages read resource whose Host contract requires a minimum release.
func RequiresMessagesCapabilityMinimum(manifest *Manifest) bool {
	return manifest != nil && manifest.Capabilities.CanRead("messages")
}

// RequiresSessionActivityCapabilityMinimum reports whether a manifest opts
// into the session activity read resource.
func RequiresSessionActivityCapabilityMinimum(manifest *Manifest) bool {
	return manifest != nil && manifest.Capabilities.CanRead("session_activity")
}

func (m *Manifest) validateCapabilityMinimumVersions() []error {
	if m == nil {
		return nil
	}
	var errs []error
	minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
	if RequiresMessagesCapabilityMinimum(m) && (!valid || CompareVersions(minimum, MinimumMessagesCapabilityVersion) < 0) {
		errs = append(errs, fmt.Errorf(
			"api_read resource %q requires min_kandev_version >= %s",
			"messages",
			MinimumMessagesCapabilityVersion,
		))
	}
	if RequiresSessionActivityCapabilityMinimum(m) && (!valid || CompareVersions(minimum, MinimumSessionActivityVersion) < 0) {
		errs = append(errs, fmt.Errorf(
			"api_read resource %q requires min_kandev_version >= %s",
			"session_activity",
			MinimumSessionActivityVersion,
		))
	}
	return errs
}
