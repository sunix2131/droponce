package application

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"droponce/internal/broker"
	"droponce/internal/infrastructure/database"
)

func TestDirectTransferViaEncryptedBrokerFallback(t *testing.T) {
	ctx := context.Background()
	db, dbPath, err := database.Open(ctx, t.TempDir())
	require.NoError(t, err)
	service, err := NewService(ctx, db, dbPath)
	require.NoError(t, err)
	service.directDownloads = t.TempDir()

	brokerServer := broker.New(broker.Options{MaxSessionDuration: time.Minute, MaxInFlightBytes: 8 << 20})
	ts := httptest.NewServer(brokerServer.Handler())
	defer ts.Close()

	source := filepath.Join(t.TempDir(), "hello-direct.txt")
	require.NoError(t, os.WriteFile(source, []byte("hello encrypted direct transfer"), 0o600))

	created, err := service.CreateDirectTransfer(ctx, CreateDirectTransferRequest{
		SourcePath:    source,
		BrokerURL:     ts.URL,
		ExpiresIn:     10,
		MaxDownloads:  1,
		CalculateHash: false,
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ShareURL)

	incoming, err := service.AcceptDirectTransfer(ctx, created.ShareURL)
	require.NoError(t, err)
	require.Equal(t, "connecting", incoming.Status)

	require.Eventually(t, func() bool {
		items := service.ListIncomingTransfers()
		return len(items) == 1 && items[0].Status == "completed"
	}, 10*time.Second, 100*time.Millisecond)

	items := service.ListIncomingTransfers()
	require.Len(t, items, 1)
	got, err := os.ReadFile(items[0].SavedPath)
	require.NoError(t, err)
	require.Equal(t, "hello encrypted direct transfer", string(got))
}
