package store

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageMetadataFieldsDefaultAndStoredValues(t *testing.T) {
	wantDefault := []string{"session_config", "timestamp", "duration"}
	defaults := defaultUserSettings(DefaultUserID)
	field := reflect.ValueOf(defaults).Elem().FieldByName("MessageMetadataFields")
	require.True(t, field.IsValid(), "settings model must expose message metadata fields")
	require.Equal(t, wantDefault, field.Interface())
	stored, err := scanUserSettings(settingsScanner{raw: `{}`}, DefaultUserID)
	require.NoError(t, err)
	field = reflect.ValueOf(stored).Elem().FieldByName("MessageMetadataFields")
	require.Equal(t, wantDefault, field.Interface())

	stored, err = scanUserSettings(settingsScanner{raw: `{"message_metadata_fields":[]}`}, DefaultUserID)
	require.NoError(t, err)
	got := reflect.ValueOf(stored).Elem().FieldByName("MessageMetadataFields")
	require.True(t, got.IsValid())
	require.Equal(t, []string{}, got.Interface())

	stored, err = scanUserSettings(settingsScanner{raw: `{"message_metadata_fields":["ttft","unknown"]}`}, DefaultUserID)
	require.NoError(t, err)
	got = reflect.ValueOf(stored).Elem().FieldByName("MessageMetadataFields")
	require.Equal(t, []string{"ttft"}, got.Interface())

}
