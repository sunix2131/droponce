package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"droponce/internal/application"
	"droponce/internal/infrastructure/database"
)

func TestUpdateSettingsDoesNotPersistCloudPubToken(t *testing.T) {
	ctx := context.Background()
	db, dbPath, err := database.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	service, err := application.NewService(ctx, db, dbPath)
	require.NoError(t, err)

	app := &App{ctx: ctx, service: service, settings: defaultSettings()}
	request := defaultSettings()
	request.CloudPubToken = "secret-token"

	updated, err := app.UpdateSettings(request)
	require.NoError(t, err)
	require.Equal(t, "secret-token", updated.CloudPubToken)
	require.Equal(t, "secret-token", app.settings.CloudPubToken)

	raw, ok, err := service.GetSetting(ctx, "settings")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotContains(t, raw, "secret-token")
}
