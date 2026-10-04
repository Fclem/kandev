package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectTurnMetadataPreservesTiming(t *testing.T) {
	timing := map[string]interface{}{"ttft_ms": int64(125), "generation_ms": int64(875)}

	got := ProjectTurnMetadata(map[string]interface{}{
		"timing":               timing,
		"private_internal_key": "must not escape",
	})

	require.Equal(t, timing, got["timing"])
	require.NotContains(t, got, "private_internal_key")
}
