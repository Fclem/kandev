package plugins

import (
	"context"
	"testing"

	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.1
func TestSessionCodeStatsFileCountsHostAvailability(t *testing.T) {
	host := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	committed, pending := int64(5), int64(3)
	host.codeStats.stats = []*analyticsmodels.SessionCodeStats{
		{SessionID: "observed", LinesAddedCommitted: int64Ptr(8), LinesDeletedCommitted: int64Ptr(2), FilesChangedCommitted: &committed, FilesChangedPeakPending: pending},
		{SessionID: "legacy", FilesChangedCommitted: &committed},
		{SessionID: "zero", LinesAddedCommitted: int64Ptr(0), LinesDeletedCommitted: int64Ptr(0), FilesChangedCommitted: int64Ptr(0)},
	}
	stats, _, err := host.host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Len(t, stats, 3)
	require.Equal(t, &committed, stats[0].FilesChangedCommitted)
	require.Equal(t, &pending, stats[0].FilesChangedPeakPending)
	require.True(t, stats[0].CommittedLinesAvailable)
	require.Nil(t, stats[1].FilesChangedCommitted, "committed availability gates file counts too")
	require.False(t, stats[1].CommittedLinesAvailable)
	require.NotNil(t, stats[2].FilesChangedCommitted)
	require.Zero(t, *stats[2].FilesChangedCommitted)
	require.NotNil(t, stats[2].FilesChangedPeakPending)
	require.Zero(t, *stats[2].FilesChangedPeakPending)
}
