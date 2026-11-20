package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CATALOG-001.6
func TestSessionCatalogRefreshConditionalWritePostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	exerciseCatalogConditionalWrite(t, repo)
}
