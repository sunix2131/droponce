package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"droponce/internal/broker"
	"droponce/internal/direct"
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

func TestDirectReceiverRejectsIncompleteFileAndRemovesPartialOutput(t *testing.T) {
	service, brokerURL, ticket, key := startDirectReceiver(t)

	metadata, err := json.Marshal(directMetadata{FileName: "partial.txt", Size: 5})
	require.NoError(t, err)
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 1, "metadata", metadata))
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 2, "data", []byte("abc")))
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 3, "done", []byte("ok")))

	var incoming IncomingTransferDto
	require.Eventually(t, func() bool {
		items := service.ListIncomingTransfers()
		if len(items) != 1 || items[0].Status != "failed" {
			return false
		}
		incoming = items[0]
		return true
	}, 5*time.Second, 20*time.Millisecond)
	require.Contains(t, incoming.ErrorMessage, "received 3 of 5 bytes")
	require.Eventually(t, func() bool {
		return errors.Is(statError(incoming.SavedPath), os.ErrNotExist)
	}, 5*time.Second, 20*time.Millisecond)
}

func TestDirectReceiverRejectsDataBeyondDeclaredSize(t *testing.T) {
	service, brokerURL, ticket, key := startDirectReceiver(t)

	metadata, err := json.Marshal(directMetadata{FileName: "overflow.txt", Size: 2})
	require.NoError(t, err)
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 1, "metadata", metadata))
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 2, "data", []byte("abc")))

	var incoming IncomingTransferDto
	require.Eventually(t, func() bool {
		items := service.ListIncomingTransfers()
		if len(items) != 1 || items[0].Status != "failed" {
			return false
		}
		incoming = items[0]
		return true
	}, 5*time.Second, 20*time.Millisecond)
	require.Contains(t, incoming.ErrorMessage, "exceeded declared file size")
	require.Eventually(t, func() bool {
		return errors.Is(statError(incoming.SavedPath), os.ErrNotExist)
	}, 5*time.Second, 20*time.Millisecond)
}

func TestCancelDirectReceiverStopsTransferAndRemovesPartialOutput(t *testing.T) {
	service, brokerURL, ticket, key := startDirectReceiver(t)

	metadata, err := json.Marshal(directMetadata{FileName: "cancelled.txt", Size: 5})
	require.NoError(t, err)
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 1, "metadata", metadata))
	require.NoError(t, postEncrypted(context.Background(), brokerURL, ticket.SessionID, key, 2, "data", []byte("abc")))

	var savedPath string
	require.Eventually(t, func() bool {
		items := service.ListIncomingTransfers()
		if len(items) != 1 || items[0].BytesReceived != 3 {
			return false
		}
		savedPath = items[0].SavedPath
		return true
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, service.CancelDirectSession(ticket.SessionID))
	require.Eventually(t, func() bool {
		return errors.Is(statError(savedPath), os.ErrNotExist)
	}, 5*time.Second, 20*time.Millisecond)
	require.Equal(t, "cancelled", service.ListIncomingTransfers()[0].Status)
}

func startDirectReceiver(t *testing.T) (*Service, string, direct.Ticket, []byte) {
	t.Helper()
	ctx := context.Background()
	db, dbPath, err := database.Open(ctx, t.TempDir())
	require.NoError(t, err)
	service, err := NewService(ctx, db, dbPath)
	require.NoError(t, err)
	service.directDownloads = t.TempDir()

	brokerServer := broker.New(broker.Options{MaxSessionDuration: time.Minute, MaxInFlightBytes: 8 << 20})
	ts := httptest.NewServer(brokerServer.Handler())
	t.Cleanup(ts.Close)

	senderKey, err := direct.NewKeyPair()
	require.NoError(t, err)
	secret, err := direct.NewSecret()
	require.NoError(t, err)
	ticket := direct.Ticket{
		SessionID:       "test-" + filepath.Base(t.TempDir()),
		BrokerURL:       ts.URL,
		SenderPublicKey: senderKey.Public,
		PairingSecret:   secret,
		ExpiresAt:       time.Now().UTC().Add(time.Minute),
	}
	require.NoError(t, createBrokerSession(ctx, ts.URL, ticket.SessionID, ticket.ExpiresAt))
	_, err = service.AcceptDirectTransfer(ctx, ticket.String())
	require.NoError(t, err)
	receiverPublic, ok := waitForReceiverHello(ctx, ts.URL, ticket.SessionID, secret, ticket.ExpiresAt)
	require.True(t, ok)
	key, _, err := direct.DeriveKeys(senderKey.Private, receiverPublic, secret, ticket.SessionID)
	require.NoError(t, err)
	return service, ts.URL, ticket, key
}

func statError(path string) error {
	_, err := os.Stat(path)
	return err
}
