package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"droponce/internal/domain/transfer"
	"droponce/internal/infrastructure/database"
	"droponce/internal/infrastructure/filesystem"
	netinfra "droponce/internal/infrastructure/network"
	"droponce/internal/infrastructure/qr"
	"droponce/internal/receiverweb"
)

type CreateTransferRequest struct {
	SourcePath    string `json:"sourcePath"`
	BindIP        string `json:"bindIp"`
	ExpiresIn     int    `json:"expiresInMinutes"`
	MaxDownloads  int    `json:"maxDownloads"`
	CalculateHash bool   `json:"calculateHash"`
}

type CreateInternetTransferRequest struct {
	SourcePath    string `json:"sourcePath"`
	RelayURL      string `json:"relayUrl"`
	RecipientID   string `json:"recipientId"`
	ExpiresIn     int    `json:"expiresInMinutes"`
	MaxDownloads  int    `json:"maxDownloads"`
	CalculateHash bool   `json:"calculateHash"`
}

type TransferDetails struct {
	transfer.Transfer
	ShareURL           string `json:"shareUrl,omitempty"`
	RemainingDownloads int    `json:"remainingDownloads"`
	QRCodePNGBase64    string `json:"qrCodePngBase64,omitempty"`
}

type Service struct {
	repo      *database.Repository
	db        *sql.DB
	registry  *Registry
	tokens    TokenService
	resolver  netinfra.PrivateIPv4Resolver
	serversMu sync.Mutex
	servers   map[string]*serverInstance
	dbPath    string

	directMu        sync.RWMutex
	directOutgoing  map[string]*directOutgoingSession
	directIncoming  map[string]*directIncomingSession
	directDownloads string
}

func NewService(ctx context.Context, db *sql.DB, dbPath string) (*Service, error) {
	s := &Service{
		repo:     database.NewRepository(db),
		db:       db,
		registry: NewRegistry(),
		tokens:   NewTokenService(),
		servers:  map[string]*serverInstance{},
		dbPath:   dbPath,

		directOutgoing:  map[string]*directOutgoingSession{},
		directIncoming:  map[string]*directIncomingSession{},
		directDownloads: defaultDirectDownloadDir(),
	}
	if err := s.repo.MarkRestarted(ctx, time.Now().UTC()); err != nil {
		return nil, err
	}
	go s.cleanupLoop()
	return s, nil
}

func (s *Service) DBPath() string { return s.dbPath }

func (s *Service) ActiveServerCount() int {
	s.serversMu.Lock()
	defer s.serversMu.Unlock()
	return len(s.servers)
}

func (s *Service) GetSetting(ctx context.Context, key string) (string, bool, error) {
	return s.repo.GetSetting(ctx, key)
}

func (s *Service) SetSetting(ctx context.Context, key, value string) error {
	return s.repo.SetSetting(ctx, key, value)
}

func (s *Service) NetworkEndpoints() ([]transfer.NetworkEndpoint, error) {
	return s.resolver.Resolve()
}

func (s *Service) CreateTransfer(ctx context.Context, req CreateTransferRequest) (TransferDetails, error) {
	if !netinfra.IsPrivateIPv4(req.BindIP) {
		return TransferDetails{}, errors.New("bind_ip must be a private IPv4 address")
	}
	if req.ExpiresIn < 1 || req.ExpiresIn > 24*60 {
		return TransferDetails{}, errors.New("expiresInMinutes must be between 1 and 1440")
	}
	if req.MaxDownloads != 1 && req.MaxDownloads != 3 && req.MaxDownloads != 5 && req.MaxDownloads != 10 {
		return TransferDetails{}, errors.New("invalid download limit")
	}
	active, err := s.repo.ListTransfers(ctx, true)
	if err != nil {
		return TransferDetails{}, err
	}
	if len(active) >= 10 {
		return TransferDetails{}, errors.New("active transfer limit reached")
	}
	meta, err := filesystem.Inspect(req.SourcePath)
	if err != nil {
		return TransferDetails{}, fmt.Errorf("inspect file: %w", err)
	}
	token, tokenHash, err := s.tokens.NewToken()
	if err != nil {
		return TransferDetails{}, err
	}
	server, err := s.startOrGetServer(req.BindIP)
	if err != nil {
		return TransferDetails{}, err
	}
	now := time.Now().UTC()
	tr := transfer.Transfer{
		ID:                 uuid.NewString(),
		Status:             transfer.StatusActive,
		SourceFileName:     meta.Name,
		SourcePath:         meta.ResolvedPath,
		SourceSizeBytes:    meta.SizeBytes,
		SourceModifiedAt:   meta.ModifiedAt,
		BindIP:             req.BindIP,
		Port:               server.port,
		TokenHash:          tokenHash,
		MaxDownloads:       req.MaxDownloads,
		CompletedDownloads: 0,
		ExpiresAt:          now.Add(time.Duration(req.ExpiresIn) * time.Minute),
		CreatedAt:          now,
		ActivatedAt:        now,
	}
	shareURL := fmt.Sprintf("http://%s:%d/d/%s", req.BindIP, server.port, token)
	png, err := qr.PNG(shareURL)
	if err != nil {
		return TransferDetails{}, err
	}
	if err := s.repo.UpsertTransfer(ctx, tr); err != nil {
		return TransferDetails{}, err
	}
	s.registry.Add(RuntimeTransfer{TransferID: tr.ID, RawToken: token, ShareURL: shareURL, QRCodePNG: png})
	server.addTransfer(tr.ID)
	if req.CalculateHash {
		go s.calculateHash(tr.ID, tr.SourcePath)
	}
	return s.details(tr), nil
}

func (s *Service) calculateHash(id, path string) {
	hash, err := filesystem.SHA256(path)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, err := s.repo.GetTransfer(ctx, id)
	if err != nil {
		return
	}
	tr.SourceSHA256 = hash
	_ = s.repo.UpsertTransfer(ctx, tr)
}

func (s *Service) ListActive(ctx context.Context) ([]TransferDetails, error) {
	items, err := s.repo.ListTransfers(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.detailList(items), nil
}

func (s *Service) ListHistory(ctx context.Context) ([]TransferDetails, error) {
	items, err := s.repo.ListTransfers(ctx, false)
	if err != nil {
		return nil, err
	}
	filtered := items[:0]
	for _, item := range items {
		if item.Status != transfer.StatusActive && item.Status != transfer.StatusDownloading && item.Status != transfer.StatusPreparing {
			item.SourcePath = ""
			filtered = append(filtered, item)
		}
	}
	return s.detailList(filtered), nil
}

func (s *Service) Get(ctx context.Context, id string) (TransferDetails, error) {
	tr, err := s.repo.GetTransfer(ctx, id)
	if err != nil {
		return TransferDetails{}, err
	}
	return s.details(tr), nil
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	tr, err := s.repo.GetTransfer(ctx, id)
	if err != nil {
		return err
	}
	if tr.Status == transfer.StatusActive || tr.Status == transfer.StatusDownloading || tr.Status == transfer.StatusPreparing {
		tr.Status = transfer.StatusCancelled
		tr.CancelledAt = time.Now().UTC()
		tr.StoppedAt = tr.CancelledAt
	}
	if rt, ok := s.registry.Get(id); ok && rt.CancelURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rt.CancelURL, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}
	s.registry.Remove(id)
	s.removeTransferFromServers(id)
	return s.repo.UpsertTransfer(ctx, tr)
}

func (s *Service) CreateInternetTransfer(ctx context.Context, req CreateInternetTransferRequest) (TransferDetails, error) {
	if req.ExpiresIn < 1 || req.ExpiresIn > 24*60 {
		return TransferDetails{}, errors.New("expiresInMinutes must be between 1 and 1440")
	}
	if req.MaxDownloads != 1 && req.MaxDownloads != 3 && req.MaxDownloads != 5 && req.MaxDownloads != 10 {
		return TransferDetails{}, errors.New("invalid download limit")
	}
	relayURL, err := normalizeRelayURL(req.RelayURL)
	if err != nil {
		return TransferDetails{}, err
	}
	meta, err := filesystem.Inspect(req.SourcePath)
	if err != nil {
		return TransferDetails{}, fmt.Errorf("inspect file: %w", err)
	}
	file, err := os.Open(meta.ResolvedPath)
	if err != nil {
		return TransferDetails{}, err
	}
	defer file.Close()

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	go func() {
		defer pipeWriter.Close()
		defer writer.Close()
		fields := map[string]string{
			"recipient_id":       req.RecipientID,
			"expires_in_minutes": fmt.Sprintf("%d", req.ExpiresIn),
			"max_downloads":      fmt.Sprintf("%d", req.MaxDownloads),
		}
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
		}
		part, err := writer.CreateFormFile("file", meta.Name)
		if err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pipeWriter.CloseWithError(err)
		}
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+"/v1/transfers", pipeReader)
	if err != nil {
		return TransferDetails{}, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return TransferDetails{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return TransferDetails{}, fmt.Errorf("relay rejected upload: %s", strings.TrimSpace(string(body)))
	}
	var relayResp struct {
		TransferID string `json:"transferId"`
		ShareURL   string `json:"shareUrl"`
		CancelURL  string `json:"cancelUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&relayResp); err != nil {
		return TransferDetails{}, err
	}
	if relayResp.ShareURL == "" || relayResp.CancelURL == "" {
		return TransferDetails{}, errors.New("relay returned incomplete response")
	}
	png, err := qr.PNG(relayResp.ShareURL)
	if err != nil {
		return TransferDetails{}, err
	}
	now := time.Now().UTC()
	tr := transfer.Transfer{
		ID:                 uuid.NewString(),
		Status:             transfer.StatusActive,
		SourceFileName:     meta.Name,
		SourcePath:         meta.ResolvedPath,
		SourceSizeBytes:    meta.SizeBytes,
		SourceModifiedAt:   meta.ModifiedAt,
		BindIP:             "internet",
		Port:               0,
		TokenHash:          HashToken(relayResp.TransferID),
		MaxDownloads:       req.MaxDownloads,
		CompletedDownloads: 0,
		ExpiresAt:          now.Add(time.Duration(req.ExpiresIn) * time.Minute),
		CreatedAt:          now,
		ActivatedAt:        now,
	}
	if err := s.repo.UpsertTransfer(ctx, tr); err != nil {
		return TransferDetails{}, err
	}
	s.registry.Add(RuntimeTransfer{TransferID: tr.ID, RawToken: "internet:" + relayResp.TransferID, ShareURL: relayResp.ShareURL, CancelURL: relayResp.CancelURL, QRCodePNG: png})
	if req.CalculateHash {
		go s.calculateHash(tr.ID, tr.SourcePath)
	}
	return s.details(tr), nil
}

func (s *Service) DeleteHistory(ctx context.Context, id string) error {
	return s.repo.DeleteTransfer(ctx, id)
}

func (s *Service) ClearHistory(ctx context.Context) error {
	return s.repo.ClearHistory(ctx)
}

func (s *Service) QRCode(id string) ([]byte, error) {
	rt, ok := s.registry.Get(id)
	if !ok {
		return nil, errors.New("transfer is not active")
	}
	return rt.QRCodePNG, nil
}

func (s *Service) SetRuntimeShareURL(id, shareURL string) error {
	if _, err := url.ParseRequestURI(shareURL); err != nil {
		return err
	}
	if _, ok := s.registry.Get(id); !ok {
		return errors.New("transfer is not active")
	}
	return s.registry.SetShareURL(id, shareURL)
}

func (s *Service) SaveQRCode(id, dir string) (string, error) {
	png, err := s.QRCode(id)
	if err != nil {
		return "", err
	}
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "droponce-"+id+".png")
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	items, _ := s.repo.ListTransfers(ctx, true)
	now := time.Now().UTC()
	for _, tr := range items {
		tr.Status = transfer.StatusEndedAfterRestart
		tr.StoppedAt = now
		tr.SourcePath = ""
		_ = s.repo.UpsertTransfer(ctx, tr)
		s.registry.Remove(tr.ID)
	}
	s.serversMu.Lock()
	defer s.serversMu.Unlock()
	for _, srv := range s.servers {
		_ = srv.server.Shutdown(ctx)
	}
	_ = s.db.Close()
	return nil
}

func (s *Service) details(t transfer.Transfer) TransferDetails {
	d := TransferDetails{Transfer: t, RemainingDownloads: t.RemainingDownloads()}
	if rt, ok := s.registry.Get(t.ID); ok {
		d.ShareURL = rt.ShareURL
	}
	return d
}

func (s *Service) detailList(items []transfer.Transfer) []TransferDetails {
	out := make([]TransferDetails, 0, len(items))
	for _, item := range items {
		out = append(out, s.details(item))
	}
	return out
}

type serverInstance struct {
	server    *http.Server
	listener  net.Listener
	port      int
	transfers map[string]struct{}
	mu        sync.Mutex
}

func (s *Service) startOrGetServer(bindIP string) (*serverInstance, error) {
	s.serversMu.Lock()
	defer s.serversMu.Unlock()
	if existing, ok := s.servers[bindIP]; ok {
		return existing, nil
	}
	if !netinfra.IsPrivateIPv4(bindIP) {
		return nil, errors.New("refusing to listen on non-private IPv4")
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(bindIP, "0"))
	if err != nil {
		return nil, err
	}
	addr := ln.Addr().(*net.TCPAddr)
	srv := &http.Server{
		Handler:           s.httpHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	instance := &serverInstance{server: srv, listener: ln, port: addr.Port, transfers: map[string]struct{}{}}
	s.servers[bindIP] = instance
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = err
		}
	}()
	return instance, nil
}

func (s *Service) removeTransferFromServers(id string) {
	s.serversMu.Lock()
	defer s.serversMu.Unlock()
	for _, srv := range s.servers {
		srv.mu.Lock()
		delete(srv.transfers, id)
		srv.mu.Unlock()
	}
}

func (s *serverInstance) addTransfer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transfers[id] = struct{}{}
}

func (s *Service) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		items, err := s.repo.ListTransfers(ctx, true)
		if err == nil {
			now := time.Now().UTC()
			for _, tr := range items {
				if !now.Before(tr.ExpiresAt) {
					tr.Status = transfer.StatusExpired
					tr.StoppedAt = now
					_ = s.repo.UpsertTransfer(ctx, tr)
					s.registry.Remove(tr.ID)
					s.removeTransferFromServers(tr.ID)
				}
			}
		}
		cancel()
	}
}

func (s *Service) httpHandler() http.Handler {
	mux := http.NewServeMux()
	limiter := newRateLimiter()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(r.RemoteAddr) {
			netinfra.SecurityHeaders(w, true)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/app.webmanifest" {
			receiverManifest(w)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/drop-icon.svg" {
			receiverIcon(w)
			return
		}
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/d/") {
			notFound(w)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/d/"), "/")
		if len(parts) == 1 {
			s.transferPage(w, r, parts[0])
			return
		}
		if len(parts) == 2 && parts[1] == "download" {
			s.download(w, r, parts[0])
			return
		}
		notFound(w)
	})
	return mux
}

func (s *Service) transferPage(w http.ResponseWriter, r *http.Request, token string) {
	netinfra.SecurityHeaders(w, true)
	tr, ok := s.lookupRuntimeTransfer(r.Context(), token)
	if !ok {
		notFound(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = receiverweb.Render(w, receiverweb.PageData{
		Title:        "DropOnce",
		Mode:         "Передача в локальной сети",
		FileName:     tr.SourceFileName,
		SizeBytes:    tr.SourceSizeBytes,
		Expires:      tr.ExpiresAt.Local().Format("15:04 02.01.2006"),
		Remaining:    tr.RemainingDownloads(),
		DownloadPath: "/d/" + url.PathEscape(token) + "/download",
		TrustNote:    "Файл передаётся напрямую с компьютера отправителя. Страница работает, пока DropOnce открыт и телефон видит этот компьютер по сети.",
	})
}

func receiverManifest(w http.ResponseWriter) {
	netinfra.SecurityHeaders(w, false)
	body, err := receiverweb.ManifestJSON()
	if err != nil {
		http.Error(w, "manifest error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	_, _ = w.Write(body)
}

func receiverIcon(w http.ResponseWriter) {
	netinfra.SecurityHeaders(w, false)
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(receiverweb.IconSVG()))
}

func (s *Service) download(w http.ResponseWriter, r *http.Request, token string) {
	if r.Header.Get("Range") != "" {
		netinfra.SecurityHeaders(w, false)
		w.Header().Set("Accept-Ranges", "none")
		http.Error(w, "Range requests are not supported", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	rt, ok := s.registry.GetByToken(token)
	if !ok {
		notFound(w)
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fresh, _ := s.registry.Get(rt.TransferID)
	if fresh.IsDownloading {
		conflict(w)
		return
	}
	tr, ok := s.lookupRuntimeTransfer(r.Context(), token)
	if !ok {
		notFound(w)
		return
	}
	if !filesystem.Unchanged(tr.SourcePath, tr.SourceSizeBytes, tr.SourceModifiedAt) {
		s.failTransfer(r.Context(), tr, "file_changed", "file changed after transfer creation")
		notFound(w)
		return
	}
	attemptID := uuid.NewString()
	now := time.Now().UTC()
	_ = s.repo.AddAttempt(r.Context(), transfer.DownloadAttempt{ID: attemptID, TransferID: tr.ID, Status: transfer.DownloadReserved, StartedAt: now})
	tr.Status = transfer.StatusDownloading
	_ = s.repo.UpsertTransfer(r.Context(), tr)
	rt.IsDownloading = true
	s.registry.SetDownloading(tr.ID, true)
	defer func() {
		s.registry.SetDownloading(tr.ID, false)
	}()
	file, err := os.Open(tr.SourcePath)
	if err != nil {
		s.failTransfer(r.Context(), tr, "open_failed", "could not open source file")
		notFound(w)
		return
	}
	defer file.Close()
	netinfra.SecurityHeaders(w, false)
	w.Header().Set("Content-Type", contentType(tr.SourceFileName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", tr.SourceSizeBytes))
	w.Header().Set("Content-Disposition", contentDisposition(tr.SourceFileName))
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)
	counter := &countingReader{reader: file}
	_, copyErr := io.Copy(w, counter)
	done := time.Now().UTC()
	if copyErr != nil || counter.sent != tr.SourceSizeBytes {
		_ = s.repo.FinishAttempt(r.Context(), attemptID, transfer.DownloadInterrupted, counter.sent, "interrupted", "download interrupted", done)
		tr.Status = transfer.StatusActive
		_ = s.repo.UpsertTransfer(context.Background(), tr)
		return
	}
	if !filesystem.Unchanged(tr.SourcePath, tr.SourceSizeBytes, tr.SourceModifiedAt) {
		_ = s.repo.FinishAttempt(context.Background(), attemptID, transfer.DownloadFailed, counter.sent, "file_changed", "file changed while downloading", done)
		s.failTransfer(context.Background(), tr, "file_changed", "file changed while downloading")
		return
	}
	_ = s.repo.FinishAttempt(context.Background(), attemptID, transfer.DownloadCompleted, counter.sent, "", "", done)
	tr.CompletedDownloads++
	if tr.CompletedDownloads >= tr.MaxDownloads {
		tr.Status = transfer.StatusConsumed
		tr.CompletedAt = done
		tr.StoppedAt = done
		s.registry.Remove(tr.ID)
		s.removeTransferFromServers(tr.ID)
	} else {
		tr.Status = transfer.StatusActive
	}
	_ = s.repo.UpsertTransfer(context.Background(), tr)
}

func (s *Service) lookupRuntimeTransfer(ctx context.Context, token string) (transfer.Transfer, bool) {
	if _, ok := s.registry.GetByToken(token); !ok {
		return transfer.Transfer{}, false
	}
	tr, err := s.repo.GetTransferByHash(ctx, HashToken(token))
	if err != nil {
		return transfer.Transfer{}, false
	}
	if !tr.IsRuntimeActive(time.Now().UTC()) {
		return transfer.Transfer{}, false
	}
	return tr, true
}

func (s *Service) failTransfer(ctx context.Context, tr transfer.Transfer, code, msg string) {
	tr.Status = transfer.StatusFailed
	tr.LastErrorCode = code
	tr.LastErrorMessage = msg
	tr.StoppedAt = time.Now().UTC()
	_ = s.repo.UpsertTransfer(ctx, tr)
	s.registry.Remove(tr.ID)
	s.removeTransferFromServers(tr.ID)
}

func notFound(w http.ResponseWriter) {
	netinfra.SecurityHeaders(w, true)
	http.Error(w, "Эта ссылка недоступна или срок её действия закончился.", http.StatusNotFound)
}

func conflict(w http.ResponseWriter) {
	netinfra.SecurityHeaders(w, true)
	http.Error(w, "Этот файл уже скачивается. Попробуйте открыть ссылку позже.", http.StatusConflict)
}

type countingReader struct {
	reader io.Reader
	sent   int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.sent += int64(n)
	return n, err
}

func contentType(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

func contentDisposition(name string) string {
	clean := strings.NewReplacer("\r", "", "\n", "", `"`, "'").Replace(filepath.Base(name))
	ascii := clean
	for _, r := range ascii {
		if r > 126 || r < 32 {
			ascii = "download"
			break
		}
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(clean))
}

func normalizeRelayURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", errors.New("relay URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid relay URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("relay URL must start with http:// or https://")
	}
	if parsed.Scheme != "https" {
		return "", errors.New("internet relay must use https")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return "", errors.New("internet relay must be public, not localhost")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !addr.Is4() && !addr.Is6() {
			return "", errors.New("invalid relay host")
		}
		if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
			return "", errors.New("internet relay must be public, not a local network address")
		}
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rate.Limiter
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{clients: map[string]*rate.Limiter{}}
}

func (l *rateLimiter) allow(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.clients[host]
	if !ok {
		lim = rate.NewLimiter(rate.Every(time.Minute/60), 10)
		l.clients[host] = lim
	}
	return lim.Allow()
}
