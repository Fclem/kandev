package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/user/models"
)

func TestMessageMetadataFieldsDTOPreservesEmptyPatch(t *testing.T) {
	var omitted UpdateUserSettingsRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.MessageMetadataFields)

	var cleared UpdateUserSettingsRequest
	require.NoError(t, json.Unmarshal([]byte(`{"message_metadata_fields":[]}`), &cleared))
	require.NotNil(t, cleared.MessageMetadataFields)
	require.Empty(t, *cleared.MessageMetadataFields)

	response := FromUserSettings(&models.UserSettings{MessageMetadataFields: []string{}})
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Fields []string `json:"message_metadata_fields"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.Fields)
	require.Empty(t, decoded.Fields)
}
