package direct

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeriveKeysMatchOnBothSides(t *testing.T) {
	sender, err := NewKeyPair()
	require.NoError(t, err)
	receiver, err := NewKeyPair()
	require.NoError(t, err)
	secret, err := NewSecret()
	require.NoError(t, err)

	s2rA, r2sA, err := DeriveKeys(sender.Private, receiver.Public, secret, "session-1")
	require.NoError(t, err)
	s2rB, r2sB, err := DeriveKeys(receiver.Private, sender.Public, secret, "session-1")
	require.NoError(t, err)
	require.Equal(t, s2rA, s2rB)
	require.Equal(t, r2sA, r2sB)
}

func TestProofRejectsInvalidPairingSecret(t *testing.T) {
	receiver, err := NewKeyPair()
	require.NoError(t, err)
	secret, err := NewSecret()
	require.NoError(t, err)
	wrong, err := NewSecret()
	require.NoError(t, err)
	proof, err := Proof(secret, "session-1", receiver.Public)
	require.NoError(t, err)
	require.True(t, VerifyProof(secret, "session-1", receiver.Public, proof))
	require.False(t, VerifyProof(wrong, "session-1", receiver.Public, proof))
}

func TestSealOpenAndReplayGuard(t *testing.T) {
	key := make([]byte, 32)
	sealed, err := Seal(key, 1, "data", []byte("hello"))
	require.NoError(t, err)
	opened, err := Open(key, 1, "data", sealed)
	require.NoError(t, err)
	require.Equal(t, "hello", string(opened))

	guard := NewReplayGuard()
	require.NoError(t, guard.Accept(1))
	require.Error(t, guard.Accept(1))
	require.NoError(t, guard.Accept(3))
	require.Error(t, guard.Accept(2))
	require.Error(t, NewReplayGuard().Accept(0))
}

func TestTicketRoundTripAndExpiry(t *testing.T) {
	sender, err := NewKeyPair()
	require.NoError(t, err)
	secret, err := NewSecret()
	require.NoError(t, err)
	ticket := Ticket{
		SessionID:       "abc",
		BrokerURL:       "http://127.0.0.1:8091",
		SenderPublicKey: sender.Public,
		PairingSecret:   secret,
		ExpiresAt:       time.Now().Add(time.Minute),
	}
	parsed, err := ParseTicket(ticket.String(), time.Now())
	require.NoError(t, err)
	require.Equal(t, ticket.SessionID, parsed.SessionID)
	_, err = ParseTicket(ticket.String(), time.Now().Add(2*time.Minute))
	require.Error(t, err)
}
