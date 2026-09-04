package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudPubURLParsesPublishedURL(t *testing.T) {
	output := "Сервис опубликован: [DropOnce] http://127.0.0.1:9876 -> https://promptly-exalted-gar.cloudpub.ru:443"
	require.Equal(t, "https://promptly-exalted-gar.cloudpub.ru:443", cloudPubURL(output))
}

func TestCloudPubDoesNotDownloadMissingCLI(t *testing.T) {
	t.Setenv("PATH", "")
	manager := NewCloudPubManager(t.TempDir())

	_, err := manager.ensureCLI(context.Background())

	require.ErrorContains(t, err, "was not found")
	require.NoFileExists(t, filepath.Join(manager.dataDir, "cloudpub", "clo"))
}
