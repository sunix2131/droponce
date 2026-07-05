package direct

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	TicketScheme = "droponce"
	TicketHost   = "receive"
)

type KeyPair struct {
	Private *ecdh.PrivateKey
	Public  string
}

type Ticket struct {
	SessionID       string
	BrokerURL       string
	SenderPublicKey string
	PairingSecret   string
	ExpiresAt       time.Time
}

func NewKeyPair() (KeyPair, error) {
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{Private: private, Public: Encode(private.PublicKey().Bytes())}, nil
}

func NewSecret() (string, error) {
	var raw [32]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return Encode(raw[:]), nil
}

func Encode(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func Decode(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

func PublicKey(value string) (*ecdh.PublicKey, error) {
	raw, err := Decode(value)
	if err != nil {
		return nil, err
	}
	return ecdh.X25519().NewPublicKey(raw)
}

func DeriveKeys(private *ecdh.PrivateKey, peerPublic string, pairingSecret string, sessionID string) (senderToReceiver []byte, receiverToSender []byte, err error) {
	peer, err := PublicKey(peerPublic)
	if err != nil {
		return nil, nil, err
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, nil, err
	}
	secret, err := Decode(pairingSecret)
	if err != nil {
		return nil, nil, err
	}
	reader := hkdf.New(sha256.New, shared, secret, []byte("DropOnce Direct P2P "+sessionID))
	material := make([]byte, 64)
	if _, err := io.ReadFull(reader, material); err != nil {
		return nil, nil, err
	}
	return material[:32], material[32:], nil
}

func Proof(pairingSecret, sessionID, receiverPublic string) (string, error) {
	secret, err := Decode(pairingSecret)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(sessionID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(receiverPublic))
	return Encode(mac.Sum(nil)), nil
}

func VerifyProof(pairingSecret, sessionID, receiverPublic, proof string) bool {
	expected, err := Proof(pairingSecret, sessionID, receiverPublic)
	if err != nil {
		return false
	}
	expectedRaw, err := Decode(expected)
	if err != nil {
		return false
	}
	proofRaw, err := Decode(proof)
	if err != nil {
		return false
	}
	return hmac.Equal(expectedRaw, proofRaw)
}

func Seal(key []byte, counter uint64, aad string, plaintext []byte) (string, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return Encode(aead.Seal(nil, nonce, plaintext, []byte(aad))), nil
}

func Open(key []byte, counter uint64, aad string, sealed string) ([]byte, error) {
	raw, err := Decode(sealed)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return aead.Open(nil, nonce, raw, []byte(aad))
}

type ReplayGuard struct {
	mu   sync.Mutex
	seen map[uint64]struct{}
}

func NewReplayGuard() *ReplayGuard {
	return &ReplayGuard{seen: map[uint64]struct{}{}}
}

func (g *ReplayGuard) Accept(counter uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.seen[counter]; ok {
		return errors.New("direct frame replay detected")
	}
	g.seen[counter] = struct{}{}
	return nil
}

func (t Ticket) String() string {
	values := url.Values{}
	values.Set("broker", t.BrokerURL)
	values.Set("pk", t.SenderPublicKey)
	values.Set("secret", t.PairingSecret)
	values.Set("exp", strconv.FormatInt(t.ExpiresAt.Unix(), 10))
	return (&url.URL{Scheme: TicketScheme, Host: TicketHost, Path: "/" + t.SessionID, RawQuery: values.Encode()}).String()
}

func ParseTicket(value string, now time.Time) (Ticket, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return Ticket{}, err
	}
	if parsed.Scheme != TicketScheme || parsed.Host != TicketHost {
		return Ticket{}, errors.New("not a DropOnce direct ticket")
	}
	expUnix, err := strconv.ParseInt(parsed.Query().Get("exp"), 10, 64)
	if err != nil {
		return Ticket{}, errors.New("invalid direct ticket expiry")
	}
	sessionID := strings.TrimPrefix(parsed.Path, "/")
	ticket := Ticket{
		SessionID:       sessionID,
		BrokerURL:       parsed.Query().Get("broker"),
		SenderPublicKey: parsed.Query().Get("pk"),
		PairingSecret:   parsed.Query().Get("secret"),
		ExpiresAt:       time.Unix(expUnix, 0).UTC(),
	}
	if ticket.SessionID == "" || ticket.BrokerURL == "" || ticket.SenderPublicKey == "" || ticket.PairingSecret == "" {
		return Ticket{}, errors.New("incomplete direct ticket")
	}
	if !now.Before(ticket.ExpiresAt) {
		return Ticket{}, errors.New("direct ticket has expired")
	}
	if _, err := PublicKey(ticket.SenderPublicKey); err != nil {
		return Ticket{}, fmt.Errorf("invalid sender public key: %w", err)
	}
	if raw, err := Decode(ticket.PairingSecret); err != nil || len(raw) != 32 {
		return Ticket{}, errors.New("invalid pairing secret")
	}
	return ticket, nil
}
