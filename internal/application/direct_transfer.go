package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"droponce/internal/broker"
	"droponce/internal/direct"
	"droponce/internal/domain/transfer"
	"droponce/internal/infrastructure/filesystem"
	"droponce/internal/infrastructure/qr"
)

const directChunkSize = 256 * 1024

type CreateDirectTransferRequest struct {
	SourcePath    string `json:"sourcePath"`
	BrokerURL     string `json:"brokerUrl"`
	ExpiresIn     int    `json:"expiresInMinutes"`
	MaxDownloads  int    `json:"maxDownloads"`
	CalculateHash bool   `json:"calculateHash"`
}

type IncomingTransferDto struct {
	SessionID     string    `json:"sessionId"`
	Status        string    `json:"status"`
	FileName      string    `json:"fileName,omitempty"`
	SizeBytes     int64     `json:"sizeBytes,omitempty"`
	BytesReceived int64     `json:"bytesReceived"`
	SavedPath     string    `json:"savedPath,omitempty"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
	CompletedAt   time.Time `json:"completedAt,omitempty"`
}

type directOutgoingSession struct {
	sessionID  string
	transferID string
	cancel     context.CancelFunc
	status     atomic.Value
}

type directReceiverHello struct {
	PublicKey string `json:"publicKey"`
	Proof     string `json:"proof"`
}

type encryptedPayload struct {
	Counter uint64 `json:"counter"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
}

type directMetadata struct {
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
}

func (s *Service) CreateDirectTransfer(ctx context.Context, req CreateDirectTransferRequest) (TransferDetails, error) {
	if req.ExpiresIn < 1 || req.ExpiresIn > 24*60 {
		return TransferDetails{}, errors.New("expiresInMinutes must be between 1 and 1440")
	}
	if req.MaxDownloads != 1 {
		return TransferDetails{}, errors.New("direct p2p currently supports a single completed download")
	}
	brokerURL, err := normalizeDirectBrokerURL(req.BrokerURL)
	if err != nil {
		return TransferDetails{}, err
	}
	meta, err := filesystem.Inspect(req.SourcePath)
	if err != nil {
		return TransferDetails{}, fmt.Errorf("inspect file: %w", err)
	}
	keyPair, err := direct.NewKeyPair()
	if err != nil {
		return TransferDetails{}, err
	}
	secret, err := direct.NewSecret()
	if err != nil {
		return TransferDetails{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(req.ExpiresIn) * time.Minute)
	sessionID := uuid.NewString()
	if err := createBrokerSession(ctx, brokerURL, sessionID, expiresAt); err != nil {
		return TransferDetails{}, err
	}
	ticket := direct.Ticket{
		SessionID:       sessionID,
		BrokerURL:       brokerURL,
		SenderPublicKey: keyPair.Public,
		PairingSecret:   secret,
		ExpiresAt:       expiresAt,
	}
	ticketString := ticket.String()
	png, err := qr.PNG(ticketString)
	if err != nil {
		return TransferDetails{}, err
	}
	tr := transfer.Transfer{
		ID:                 uuid.NewString(),
		Status:             transfer.StatusActive,
		SourceFileName:     meta.Name,
		SourcePath:         meta.ResolvedPath,
		SourceSizeBytes:    meta.SizeBytes,
		SourceModifiedAt:   meta.ModifiedAt,
		BindIP:             "direct-p2p",
		TokenHash:          HashToken(sessionID),
		MaxDownloads:       1,
		CompletedDownloads: 0,
		ExpiresAt:          expiresAt,
		CreatedAt:          now,
		ActivatedAt:        now,
	}
	if err := s.repo.UpsertTransfer(ctx, tr); err != nil {
		return TransferDetails{}, err
	}
	s.registry.Add(RuntimeTransfer{TransferID: tr.ID, RawToken: "direct:" + sessionID, ShareURL: ticketString, QRCodePNG: png})
	runCtx, cancel := context.WithCancel(context.Background())
	outgoing := &directOutgoingSession{sessionID: sessionID, transferID: tr.ID, cancel: cancel}
	outgoing.status.Store("waiting_for_receiver")
	s.directMu.Lock()
	s.directOutgoing[sessionID] = outgoing
	s.directMu.Unlock()
	go s.runDirectSender(runCtx, tr, brokerURL, keyPair, secret)
	if req.CalculateHash {
		go s.calculateHash(tr.ID, tr.SourcePath)
	}
	return s.details(tr), nil
}

func (s *Service) AcceptDirectTransfer(ctx context.Context, ticketValue string) (IncomingTransferDto, error) {
	ticket, err := direct.ParseTicket(ticketValue, time.Now().UTC())
	if err != nil {
		return IncomingTransferDto{}, err
	}
	brokerURL, err := normalizeDirectBrokerURL(ticket.BrokerURL)
	if err != nil {
		return IncomingTransferDto{}, err
	}
	keyPair, err := direct.NewKeyPair()
	if err != nil {
		return IncomingTransferDto{}, err
	}
	proof, err := direct.Proof(ticket.PairingSecret, ticket.SessionID, keyPair.Public)
	if err != nil {
		return IncomingTransferDto{}, err
	}
	hello, _ := json.Marshal(directReceiverHello{PublicKey: keyPair.Public, Proof: proof})
	if err := postBrokerMessage(ctx, brokerURL, ticket.SessionID, "receiver", "sender", "receiver_hello", direct.Encode(hello)); err != nil {
		return IncomingTransferDto{}, err
	}
	dto := IncomingTransferDto{SessionID: ticket.SessionID, Status: "connecting", StartedAt: time.Now().UTC()}
	s.directMu.Lock()
	s.directIncoming[ticket.SessionID] = &dto
	s.directMu.Unlock()
	go s.runDirectReceiver(context.Background(), ticket, brokerURL, keyPair)
	return dto, nil
}

func (s *Service) ListIncomingTransfers() []IncomingTransferDto {
	s.directMu.RLock()
	defer s.directMu.RUnlock()
	out := make([]IncomingTransferDto, 0, len(s.directIncoming))
	for _, item := range s.directIncoming {
		out = append(out, *item)
	}
	return out
}

func (s *Service) CancelDirectSession(sessionID string) error {
	s.directMu.Lock()
	defer s.directMu.Unlock()
	if outgoing, ok := s.directOutgoing[sessionID]; ok {
		outgoing.cancel()
		delete(s.directOutgoing, sessionID)
	}
	if incoming, ok := s.directIncoming[sessionID]; ok {
		incoming.Status = "cancelled"
		incoming.CompletedAt = time.Now().UTC()
	}
	return nil
}

func (s *Service) runDirectSender(ctx context.Context, tr transfer.Transfer, brokerURL string, keyPair direct.KeyPair, secret string) {
	sessionID := HashToken(tr.TokenHash)
	for _, rt := range s.registry.List() {
		if rt.TransferID == tr.ID {
			sessionID = strings.TrimPrefix(rt.RawToken, "direct:")
			break
		}
	}
	outgoing := s.outgoing(sessionID)
	if outgoing == nil {
		return
	}
	receiverPublic, ok := waitForReceiverHello(ctx, brokerURL, sessionID, secret, tr.ExpiresAt)
	if !ok {
		s.failTransfer(context.Background(), tr, "direct_timeout", "receiver did not join direct session")
		return
	}
	outgoing.status.Store("encrypted_bridge")
	s2r, _, err := direct.DeriveKeys(keyPair.Private, receiverPublic, secret, sessionID)
	if err != nil {
		s.failTransfer(context.Background(), tr, "direct_crypto_failed", "could not derive direct session keys")
		return
	}
	if !filesystem.Unchanged(tr.SourcePath, tr.SourceSizeBytes, tr.SourceModifiedAt) {
		s.failTransfer(context.Background(), tr, "file_changed", "file changed after transfer creation")
		return
	}
	file, err := os.Open(tr.SourcePath)
	if err != nil {
		s.failTransfer(context.Background(), tr, "open_failed", "could not open source file")
		return
	}
	defer file.Close()
	tr.Status = transfer.StatusDownloading
	_ = s.repo.UpsertTransfer(context.Background(), tr)
	counter := uint64(1)
	meta, _ := json.Marshal(directMetadata{FileName: tr.SourceFileName, Size: tr.SourceSizeBytes})
	if err := postEncrypted(ctx, brokerURL, sessionID, s2r, counter, "metadata", meta); err != nil {
		s.failTransfer(context.Background(), tr, "direct_send_failed", err.Error())
		return
	}
	counter++
	buf := make([]byte, directChunkSize)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := postEncrypted(ctx, brokerURL, sessionID, s2r, counter, "data", buf[:n]); err != nil {
				s.failTransfer(context.Background(), tr, "direct_send_failed", err.Error())
				return
			}
			counter++
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			s.failTransfer(context.Background(), tr, "direct_read_failed", readErr.Error())
			return
		}
	}
	if !filesystem.Unchanged(tr.SourcePath, tr.SourceSizeBytes, tr.SourceModifiedAt) {
		s.failTransfer(context.Background(), tr, "file_changed", "file changed while downloading")
		return
	}
	if err := postEncrypted(ctx, brokerURL, sessionID, s2r, counter, "done", []byte("ok")); err != nil {
		s.failTransfer(context.Background(), tr, "direct_send_failed", err.Error())
		return
	}
	now := time.Now().UTC()
	tr.Status = transfer.StatusConsumed
	tr.CompletedDownloads = 1
	tr.CompletedAt = now
	tr.StoppedAt = now
	_ = s.repo.UpsertTransfer(context.Background(), tr)
	s.registry.Remove(tr.ID)
	s.directMu.Lock()
	delete(s.directOutgoing, sessionID)
	s.directMu.Unlock()
}

func (s *Service) runDirectReceiver(ctx context.Context, ticket direct.Ticket, brokerURL string, keyPair direct.KeyPair) {
	s2r, _, err := direct.DeriveKeys(keyPair.Private, ticket.SenderPublicKey, ticket.PairingSecret, ticket.SessionID)
	if err != nil {
		s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
			dto.Status = "failed"
			dto.ErrorMessage = err.Error()
			dto.CompletedAt = time.Now().UTC()
		})
		return
	}
	guard := direct.NewReplayGuard()
	after := uint64(0)
	var out *os.File
	var received int64
	for time.Now().UTC().Before(ticket.ExpiresAt) {
		messages, err := pollBrokerMessages(ctx, brokerURL, ticket.SessionID, "receiver", after)
		if err != nil {
			s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
				dto.Status = "failed"
				dto.ErrorMessage = err.Error()
				dto.CompletedAt = time.Now().UTC()
			})
			return
		}
		for _, msg := range messages {
			if msg.Seq > after {
				after = msg.Seq
			}
			if msg.Type != "encrypted" {
				continue
			}
			payload, plaintext, err := openEncrypted(s2r, guard, msg.Body)
			if err != nil {
				s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
					dto.Status = "failed"
					dto.ErrorMessage = err.Error()
					dto.CompletedAt = time.Now().UTC()
				})
				return
			}
			switch payload.Kind {
			case "metadata":
				var meta directMetadata
				if err := json.Unmarshal(plaintext, &meta); err != nil {
					return
				}
				path, err := s.prepareIncomingFile(meta.FileName)
				if err != nil {
					s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
						dto.Status = "failed"
						dto.ErrorMessage = err.Error()
						dto.CompletedAt = time.Now().UTC()
					})
					return
				}
				out, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
						dto.Status = "failed"
						dto.ErrorMessage = err.Error()
						dto.CompletedAt = time.Now().UTC()
					})
					return
				}
				s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
					dto.Status = "receiving"
					dto.FileName = meta.FileName
					dto.SizeBytes = meta.Size
					dto.SavedPath = path
				})
			case "data":
				if out == nil {
					continue
				}
				n, err := out.Write(plaintext)
				received += int64(n)
				if err != nil {
					_ = out.Close()
					s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
						dto.Status = "failed"
						dto.ErrorMessage = err.Error()
						dto.CompletedAt = time.Now().UTC()
					})
					return
				}
				s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
					dto.BytesReceived = received
				})
			case "done":
				if out != nil {
					_ = out.Close()
				}
				s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
					dto.Status = "completed"
					dto.BytesReceived = received
					dto.CompletedAt = time.Now().UTC()
				})
				return
			}
		}
	}
	s.updateIncoming(ticket.SessionID, func(dto *IncomingTransferDto) {
		dto.Status = "expired"
		dto.CompletedAt = time.Now().UTC()
	})
}

func (s *Service) outgoing(sessionID string) *directOutgoingSession {
	s.directMu.RLock()
	defer s.directMu.RUnlock()
	return s.directOutgoing[sessionID]
}

func (s *Service) updateIncoming(sessionID string, fn func(*IncomingTransferDto)) {
	s.directMu.Lock()
	defer s.directMu.Unlock()
	if dto, ok := s.directIncoming[sessionID]; ok {
		fn(dto)
	}
}

func (s *Service) prepareIncomingFile(name string) (string, error) {
	if err := os.MkdirAll(s.directDownloads, 0o700); err != nil {
		return "", err
	}
	clean := strings.NewReplacer("\r", "", "\n", "", string(filepath.Separator), "-").Replace(filepath.Base(name))
	if clean == "" || clean == "." {
		clean = "download"
	}
	path := filepath.Join(s.directDownloads, clean)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	ext := filepath.Ext(clean)
	base := strings.TrimSuffix(clean, ext)
	return filepath.Join(s.directDownloads, base+"-"+uuid.NewString()+ext), nil
}

func defaultDirectDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, "Downloads", "DropOnce")
}

func waitForReceiverHello(ctx context.Context, brokerURL, sessionID, secret string, expiresAt time.Time) (string, bool) {
	after := uint64(0)
	for time.Now().UTC().Before(expiresAt) {
		messages, err := pollBrokerMessages(ctx, brokerURL, sessionID, "sender", after)
		if err != nil {
			return "", false
		}
		for _, msg := range messages {
			if msg.Seq > after {
				after = msg.Seq
			}
			if msg.Type != "receiver_hello" {
				continue
			}
			raw, err := direct.Decode(msg.Body)
			if err != nil {
				continue
			}
			var hello directReceiverHello
			if err := json.Unmarshal(raw, &hello); err != nil {
				continue
			}
			if direct.VerifyProof(secret, sessionID, hello.PublicKey, hello.Proof) {
				return hello.PublicKey, true
			}
		}
	}
	return "", false
}

func createBrokerSession(ctx context.Context, brokerURL, sessionID string, expiresAt time.Time) error {
	body, _ := json.Marshal(broker.CreateSessionRequest{SessionID: sessionID, ExpiresAt: expiresAt.Unix()})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brokerURL+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("broker rejected session: %s", resp.Status)
	}
	return nil
}

func postBrokerMessage(ctx context.Context, brokerURL, sessionID, from, to, typ, body string) error {
	payload, _ := json.Marshal(broker.PostMessageRequest{From: from, To: to, Type: typ, Body: body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brokerURL+"/v1/sessions/"+url.PathEscape(sessionID)+"/messages", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("broker rejected message: %s", resp.Status)
	}
	return nil
}

func pollBrokerMessages(ctx context.Context, brokerURL, sessionID, to string, after uint64) ([]broker.Message, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/sessions/%s/messages?to=%s&after=%d", brokerURL, url.PathEscape(sessionID), url.QueryEscape(to), after), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("broker poll failed: %s", resp.Status)
	}
	var listed broker.ListMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, err
	}
	return listed.Messages, nil
}

func postEncrypted(ctx context.Context, brokerURL, sessionID string, key []byte, counter uint64, kind string, plaintext []byte) error {
	body, err := direct.Seal(key, counter, kind, plaintext)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(encryptedPayload{Counter: counter, Kind: kind, Body: body})
	return postBrokerMessage(ctx, brokerURL, sessionID, "sender", "receiver", "encrypted", direct.Encode(payload))
}

func openEncrypted(key []byte, guard *direct.ReplayGuard, body string) (encryptedPayload, []byte, error) {
	raw, err := direct.Decode(body)
	if err != nil {
		return encryptedPayload{}, nil, err
	}
	var payload encryptedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return encryptedPayload{}, nil, err
	}
	if err := guard.Accept(payload.Counter); err != nil {
		return encryptedPayload{}, nil, err
	}
	plaintext, err := direct.Open(key, payload.Counter, payload.Kind, payload.Body)
	if err != nil {
		return encryptedPayload{}, nil, err
	}
	return payload, plaintext, nil
}

func normalizeDirectBrokerURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", errors.New("broker URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid broker URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("broker URL must start with http:// or https://")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
