package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/user/models"
)

func TestApplyMessageMetadataFieldsValidatesAndPreservesSelection(t *testing.T) {
	tests := []struct {
		name    string
		fields  []string
		wantErr string
	}{
		{name: "unknown", fields: []string{"timestamp", "future"}, wantErr: "message_metadata_fields"},
		{name: "duplicate", fields: []string{"ttft", "ttft"}, wantErr: "message_metadata_fields"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := &models.UserSettings{MessageMetadataFields: []string{"timestamp"}}
			err := applyBasicSettings(settings, &UpdateUserSettingsRequest{MessageMetadataFields: &test.fields})
			require.ErrorContains(t, err, test.wantErr)
			require.Equal(t, []string{"timestamp"}, settings.MessageMetadataFields)
		})
	}

	settings := &models.UserSettings{MessageMetadataFields: []string{"timestamp"}}
	empty := []string{}
	require.NoError(t, applyBasicSettings(settings, &UpdateUserSettingsRequest{MessageMetadataFields: &empty}))
	require.NotNil(t, settings.MessageMetadataFields)
	require.Empty(t, settings.MessageMetadataFields)
}
