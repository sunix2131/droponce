package relay

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	netinfra "droponce/internal/infrastructure/network"
	"droponce/internal/receiverweb"
)

type Server struct {
	mu             sync.Mutex
	storage        string
	publicURL      string
	maxUploadBytes int64
	items          map[string]*item
}

type item struct {
	ID                 string
	Token              string
	CancelToken        string
	RecipientID        string
	FileName           string
	Path               string
	Size               int64
	MaxDownloads       int
	CompletedDownloads int
	ActiveDownloads    int
	ExpiresAt          time.Time
	CreatedAt          time.Time
}

type createResponse struct {
	TransferID string `json:"transferId"`
	ShareURL   string `json:"shareUrl"`
	CancelURL  string `json:"cancelUrl"`
}

const DefaultMaxUploadBytes int64 = 50 * 1024 * 1024 * 1024

var errUploadLimitExceeded = errors.New("upload limit exceeded")

type Options struct {
	Storage        string
	PublicURL      string
	MaxUploadBytes int64
}

func New(storage, publicURL string) (*Server, error) {
	return NewWithOptions(Options{Storage: storage, PublicURL: publicURL, MaxUploadBytes: DefaultMaxUploadBytes})
}

func NewWithOptions(options Options) (*Server, error) {
	storage := options.Storage
	if storage == "" {
		storage = filepath.Join(os.TempDir(), "droponce-relay")
	}
	if err := os.MkdirAll(storage, 0o700); err != nil {
		return nil, err
	}
	maxUploadBytes := options.MaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = DefaultMaxUploadBytes
	}
	server := &Server{storage: storage, publicURL: strings.TrimRight(options.PublicURL, "/"), maxUploadBytes: maxUploadBytes, items: map[string]*item{}}
	go server.cleanupExpiredLoop()
	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transfers", s.createTransfer)
	mux.HandleFunc("/v1/transfers/", s.manageTransfer)
	mux.HandleFunc("/app.webmanifest", relayReceiverManifest)
	mux.HandleFunc("/drop-icon.svg", relayReceiverIcon)
	mux.HandleFunc("/r/", s.receiver)
	return mux
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+(2<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	id := uuid.NewString()
	token, err := randomToken()
	if err != nil {
		http.Error(w, "token error", http.StatusInternalServerError)
		return
	}
	cancelToken, err := randomToken()
	if err != nil {
		http.Error(w, "token error", http.StatusInternalServerError)
		return
	}
	path := filepath.Join(s.storage, id+".blob")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	var size int64
	var filename string
	fileSeen := false
	fields := map[string]string{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = os.Remove(path)
			http.Error(w, "upload failed", http.StatusBadRequest)
			return
		}
		name := part.FormName()
		if name == "file" {
			if fileSeen {
				_ = os.Remove(path)
				http.Error(w, "only one file is allowed", http.StatusBadRequest)
				return
			}
			fileSeen = true
			filename = sanitizeFilename(part.FileName())
			limited := &limitedWriter{writer: out, remaining: s.maxUploadBytes}
			written, err := io.Copy(limited, part)
			size += written
			if limited.exceeded {
				_ = os.Remove(path)
				http.Error(w, "file exceeds relay limit", http.StatusRequestEntityTooLarge)
				return
			}
			if err != nil {
				_ = os.Remove(path)
				http.Error(w, "upload failed", http.StatusBadRequest)
				return
			}
			continue
		}
		if name != "" {
			value, err := io.ReadAll(io.LimitReader(part, 4096))
			if err != nil {
				_ = os.Remove(path)
				http.Error(w, "upload failed", http.StatusBadRequest)
				return
			}
			fields[name] = string(value)
		}
	}
	if filename == "" {
		_ = os.Remove(path)
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(path)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	maxDownloads, err := parseLimitedInt(fields["max_downloads"], 1, 10)
	if err != nil {
		_ = os.Remove(path)
		http.Error(w, "invalid max_downloads", http.StatusBadRequest)
		return
	}
	expiresIn, err := parseLimitedInt(fields["expires_in_minutes"], 1, 24*60)
	if err != nil {
		_ = os.Remove(path)
		http.Error(w, "invalid expires_in_minutes", http.StatusBadRequest)
		return
	}
	it := &item{
		ID:           id,
		Token:        token,
		CancelToken:  cancelToken,
		RecipientID:  cleanID(fields["recipient_id"]),
		FileName:     filename,
		Path:         path,
		Size:         size,
		MaxDownloads: maxDownloads,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(expiresIn) * time.Minute),
		CreatedAt:    time.Now().UTC(),
	}
	s.mu.Lock()
	s.items[id] = it
	s.mu.Unlock()
	base := s.requestBase(r)
	resp := createResponse{
		TransferID: id,
		ShareURL:   fmt.Sprintf("%s/r/%s/%s", base, url.PathEscape(id), url.PathEscape(token)),
		CancelURL:  fmt.Sprintf("%s/v1/transfers/%s?cancel_token=%s", base, url.PathEscape(id), url.QueryEscape(cancelToken)),
	}
	writeJSON(w, resp)
}

func (s *Server) manageTransfer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/transfers/")
	if r.Method != http.MethodDelete || id == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.mu.Lock()
	it, ok := s.items[id]
	if !ok || it.CancelToken != r.URL.Query().Get("cancel_token") {
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(s.items, id)
	s.mu.Unlock()
	_ = os.Remove(it.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) receiver(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/r/"), "/")
	if len(parts) < 2 {
		notFound(w)
		return
	}
	id, token := parts[0], parts[1]
	if len(parts) == 2 && r.Method == http.MethodGet {
		it, ok := s.lookup(id, token)
		if !ok {
			notFound(w)
			return
		}
		netinfra.SecurityHeaders(w, true)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = receiverweb.Render(w, receiverweb.PageData{
			Title:        "DropOnce Internet",
			Mode:         "Передача через relay",
			FileName:     it.FileName,
			SizeBytes:    it.Size,
			Expires:      it.ExpiresAt.Local().Format("15:04 02.01.2006"),
			Remaining:    it.MaxDownloads - it.CompletedDownloads,
			DownloadPath: "/r/" + url.PathEscape(id) + "/" + url.PathEscape(token) + "/download",
			TrustNote:    "Файл скачивается через выбранный relay. Используйте только свой или доверенный relay, потому что он временно хранит файл до скачивания, истечения срока или отмены.",
		})
		return
	}
	if len(parts) == 3 && parts[2] == "download" && r.Method == http.MethodGet {
		it, ok := s.reserveDownload(id, token)
		if !ok {
			notFound(w)
			return
		}
		s.download(w, r, it)
		return
	}
	notFound(w)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request, it *item) {
	success := false
	defer func() { s.finishDownload(it.ID, success) }()
	if r.Header.Get("Range") != "" {
		netinfra.SecurityHeaders(w, false)
		w.Header().Set("Accept-Ranges", "none")
		http.Error(w, "Range requests are not supported", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	file, err := os.Open(it.Path)
	if err != nil {
		notFound(w)
		return
	}
	defer file.Close()
	netinfra.SecurityHeaders(w, false)
	w.Header().Set("Content-Type", contentType(it.FileName))
	w.Header().Set("Content-Length", strconv.FormatInt(it.Size, 10))
	w.Header().Set("Content-Disposition", contentDisposition(it.FileName))
	w.Header().Set("Accept-Ranges", "none")
	counter := &countingReader{reader: file}
	if _, err := io.Copy(w, counter); err != nil || counter.sent != it.Size {
		return
	}
	success = true
}

func (s *Server) reserveDownload(id, token string) (*item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || it.Token != token || !time.Now().UTC().Before(it.ExpiresAt) {
		return nil, false
	}
	if it.CompletedDownloads+it.ActiveDownloads >= it.MaxDownloads {
		return nil, false
	}
	it.ActiveDownloads++
	copy := *it
	return &copy, true
}

func (s *Server) finishDownload(id string, success bool) {
	var removePath string
	s.mu.Lock()
	it, ok := s.items[id]
	if ok {
		if it.ActiveDownloads > 0 {
			it.ActiveDownloads--
		}
		if success {
			it.CompletedDownloads++
			if it.CompletedDownloads >= it.MaxDownloads {
				delete(s.items, id)
				removePath = it.Path
			}
		}
	}
	s.mu.Unlock()
	if removePath != "" {
		_ = os.Remove(removePath)
	}
}

func (s *Server) lookup(id, token string) (*item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || it.Token != token || !time.Now().UTC().Before(it.ExpiresAt) || it.CompletedDownloads+it.ActiveDownloads >= it.MaxDownloads {
		if ok && !time.Now().UTC().Before(it.ExpiresAt) {
			delete(s.items, id)
			_ = os.Remove(it.Path)
		}
		return nil, false
	}
	copy := *it
	return &copy, true
}

func (s *Server) cleanupExpiredLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().UTC()
		var paths []string
		s.mu.Lock()
		for id, it := range s.items {
			if !now.Before(it.ExpiresAt) {
				paths = append(paths, it.Path)
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}
}

func (s *Server) requestBase(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func parseLimitedInt(value string, min, max int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, errors.New("out of range")
	}
	return n, nil
}

func cleanID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.NewReplacer("\r", "", "\n", "", `"`, "'").Replace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "download"
	}
	return name
}

func contentType(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

func contentDisposition(name string) string {
	clean := sanitizeFilename(name)
	ascii := clean
	for _, r := range ascii {
		if r > 126 || r < 32 {
			ascii = "download"
			break
		}
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(clean))
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func notFound(w http.ResponseWriter) {
	netinfra.SecurityHeaders(w, true)
	http.Error(w, "Эта интернет-ссылка недоступна или срок её действия закончился.", http.StatusNotFound)
}

func relayReceiverManifest(w http.ResponseWriter, _ *http.Request) {
	netinfra.SecurityHeaders(w, false)
	body, err := receiverweb.ManifestJSON()
	if err != nil {
		http.Error(w, "manifest error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	_, _ = w.Write(body)
}

func relayReceiverIcon(w http.ResponseWriter, _ *http.Request) {
	netinfra.SecurityHeaders(w, false)
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(receiverweb.IconSVG()))
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

type limitedWriter struct {
	writer    io.Writer
	remaining int64
	exceeded  bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		w.exceeded = true
		return 0, errUploadLimitExceeded
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}
