package events

const SessionCatalogRefreshUpdated = "session.catalog_refresh_updated"

func BuildSessionCatalogRefreshSubject(sessionID string) string {
	return SessionCatalogRefreshUpdated + "." + sessionID
}

func BuildSessionCatalogRefreshWildcardSubject() string {
	return SessionCatalogRefreshUpdated + ".*"
}

const SessionCatalogDiscoveryUpdated = "session.catalog_discovery_updated"

func BuildSessionCatalogDiscoverySubject(sessionID string) string {
	return SessionCatalogDiscoveryUpdated + "." + sessionID
}

func BuildSessionCatalogDiscoveryWildcardSubject() string {
	return SessionCatalogDiscoveryUpdated + ".*"
}
