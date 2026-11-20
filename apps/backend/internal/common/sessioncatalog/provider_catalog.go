package sessioncatalog

// ProviderMetadataField belongs to the ACP snapshot, independently of host
// notices. A new host probe cannot erase the current provider's advertisement.
const ProviderMetadataField = "catalog_discovery"

// ProviderCatalog contains availability evidence, never effective selections.
type ProviderCatalog struct {
	Revision  uint64    `json:"revision"`
	Owner     Owner     `json:"owner"`
	ProfileID string    `json:"profile_id"`
	Result    Discovery `json:"result"`
}

// ProviderUpdate is scoped to the session whose source was committed.
type ProviderUpdate struct {
	TaskID    string          `json:"task_id"`
	SessionID string          `json:"session_id"`
	Provider  ProviderCatalog `json:"provider"`
}

func (u ProviderUpdate) GetSessionID() string { return u.SessionID }
