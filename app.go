package main

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"droponce/internal/application"
	"droponce/internal/infrastructure/database"
	"droponce/internal/infrastructure/filesystem"
)

type App struct {
	ctx      context.Context
	service  *application.Service
	dbPath   string
	logPath  string
	settings SettingsDto
	cloudpub *CloudPubManager
}

type FileSelectionDto struct {
	Path           string    `json:"path"`
	Name           string    `json:"name"`
	SizeBytes      int64     `json:"sizeBytes"`
	ModifiedAt     time.Time `json:"modifiedAt"`
	IsSymlink      bool      `json:"isSymlink"`
	SymlinkWarning string    `json:"symlinkWarning,omitempty"`
}

type QrCodeDto struct {
	PNGBase64 string `json:"pngBase64"`
}

type SaveResultDto struct {
	Path string `json:"path"`
}

type SettingsDto struct {
	Language              string `json:"language"`
	Theme                 string `json:"theme"`
	DefaultRelayURL       string `json:"defaultRelayUrl"`
	DefaultExpiryMinutes  int    `json:"defaultExpiryMinutes"`
	DefaultMaxDownloads   int    `json:"defaultMaxDownloads"`
	DefaultCalculateSHA   bool   `json:"defaultCalculateSha"`
	WarnTrustedLocalOnly  bool   `json:"warnTrustedLocalOnly"`
	MaxActiveTransfers    int    `json:"maxActiveTransfers"`
	ConfirmCloseWithLinks bool   `json:"confirmCloseWithLinks"`
	CloudPubToken         string `json:"cloudPubToken"`
}

type DiagnosticsDto struct {
	Version             string `json:"version"`
	GoVersion           string `json:"goVersion"`
	WailsVersion        string `json:"wailsVersion"`
	SQLitePath          string `json:"sqlitePath"`
	LogsPath            string `json:"logsPath"`
	ActiveServerCount   int    `json:"activeServerCount"`
	ActiveTransferCount int    `json:"activeTransferCount"`
}

type TransferLimitsDto struct {
	LocalSingleFileLimitLabel   string `json:"localSingleFileLimitLabel"`
	InternetSingleFileLimitGB   int64  `json:"internetSingleFileLimitGb"`
	InternetSingleFileLimitText string `json:"internetSingleFileLimitText"`
	MultiFileSupported          bool   `json:"multiFileSupported"`
	MultiFileAdvice             string `json:"multiFileAdvice"`
}

type RelayRecommendationDto struct {
	URL        string `json:"url"`
	IsLocalLAN bool   `json:"isLocalLan"`
}

type CreateCloudPubTransferRequest struct {
	SourcePath    string `json:"sourcePath"`
	BindIP        string `json:"bindIp"`
	ExpiresIn     int    `json:"expiresInMinutes"`
	MaxDownloads  int    `json:"maxDownloads"`
	CalculateHash bool   `json:"calculateHash"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir, err := appDataDir()
	if err != nil {
		panic(err)
	}
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		panic(err)
	}
	db, dbPath, err := database.Open(ctx, dataDir)
	if err != nil {
		panic(err)
	}
	service, err := application.NewService(ctx, db, dbPath)
	if err != nil {
		panic(err)
	}
	a.service = service
	a.dbPath = dbPath
	a.logPath = logDir
	a.cloudpub = NewCloudPubManager(dataDir)
	a.settings = defaultSettings()
	if raw, ok, err := service.GetSetting(ctx, "settings"); err == nil && ok {
		_ = json.Unmarshal([]byte(raw), &a.settings)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.cloudpub != nil {
		a.cloudpub.StopAll()
	}
	if a.service != nil {
		_ = a.service.Shutdown(ctx)
	}
}

func (a *App) SelectFile() (FileSelectionDto, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Select file"})
	if err != nil {
		return FileSelectionDto{}, err
	}
	if path == "" {
		return FileSelectionDto{}, errors.New("file selection cancelled")
	}
	meta, err := filesystem.Inspect(path)
	if err != nil {
		return FileSelectionDto{}, err
	}
	dto := FileSelectionDto{
		Path:       meta.ResolvedPath,
		Name:       meta.Name,
		SizeBytes:  meta.SizeBytes,
		ModifiedAt: meta.ModifiedAt,
		IsSymlink:  meta.IsSymlink,
	}
	if meta.IsSymlink {
		dto.SymlinkWarning = "Вы выбрали символическую ссылку. Будет передано содержимое исходного файла."
	}
	return dto, nil
}

func (a *App) SelectFilesAsZip() (FileSelectionDto, error) {
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Select files"})
	if err != nil {
		return FileSelectionDto{}, err
	}
	if len(paths) == 0 {
		return FileSelectionDto{}, errors.New("file selection cancelled")
	}
	cache, err := appCacheDir()
	if err != nil {
		return FileSelectionDto{}, err
	}
	zipPath := filepath.Join(cache, "droponce-bundle-"+uuid.NewString()+".zip")
	if err := zipFiles(zipPath, paths); err != nil {
		_ = os.Remove(zipPath)
		return FileSelectionDto{}, err
	}
	meta, err := filesystem.Inspect(zipPath)
	if err != nil {
		return FileSelectionDto{}, err
	}
	return FileSelectionDto{
		Path:           meta.ResolvedPath,
		Name:           meta.Name,
		SizeBytes:      meta.SizeBytes,
		ModifiedAt:     meta.ModifiedAt,
		SymlinkWarning: "Выбрано несколько файлов. DropOnce упаковал их в ZIP-архив для передачи одной ссылкой.",
	}, nil
}

func (a *App) GetAvailableNetworkEndpoints() (any, error) {
	return a.service.NetworkEndpoints()
}

func (a *App) CreateTransfer(request application.CreateTransferRequest) (application.TransferDetails, error) {
	return a.service.CreateTransfer(a.ctx, request)
}

func (a *App) CreateInternetTransfer(request application.CreateInternetTransferRequest) (application.TransferDetails, error) {
	return a.service.CreateInternetTransfer(a.ctx, request)
}

func (a *App) CreateCloudPubTransfer(request CreateCloudPubTransferRequest) (application.TransferDetails, error) {
	if strings.TrimSpace(a.settings.CloudPubToken) == "" {
		return application.TransferDetails{}, errors.New("CloudPub token is empty. Add it in settings first")
	}
	if strings.TrimSpace(request.BindIP) == "" {
		endpoints, err := a.service.NetworkEndpoints()
		if err != nil || len(endpoints) == 0 {
			return application.TransferDetails{}, errors.New("could not choose local network address")
		}
		request.BindIP = endpoints[0].IPAddress
	}
	details, err := a.service.CreateTransfer(a.ctx, application.CreateTransferRequest{
		SourcePath:    request.SourcePath,
		BindIP:        request.BindIP,
		ExpiresIn:     request.ExpiresIn,
		MaxDownloads:  request.MaxDownloads,
		CalculateHash: request.CalculateHash,
	})
	if err != nil {
		return application.TransferDetails{}, err
	}
	target := net.JoinHostPort(details.BindIP, strconv.Itoa(details.Port))
	publicBase, err := a.cloudpub.Start(a.ctx, a.settings.CloudPubToken, details.ID, target)
	if err != nil {
		_ = a.service.Cancel(a.ctx, details.ID)
		return application.TransferDetails{}, err
	}
	localURL, err := url.Parse(details.ShareURL)
	if err != nil {
		_ = a.cloudpub.Stop(details.ID)
		_ = a.service.Cancel(a.ctx, details.ID)
		return application.TransferDetails{}, err
	}
	publicShareURL := strings.TrimRight(publicBase, "/") + localURL.EscapedPath()
	if err := a.service.SetRuntimeShareURL(details.ID, publicShareURL); err != nil {
		_ = a.cloudpub.Stop(details.ID)
		_ = a.service.Cancel(a.ctx, details.ID)
		return application.TransferDetails{}, err
	}
	return a.service.Get(a.ctx, details.ID)
}

func (a *App) CreateDirectTransfer(request application.CreateDirectTransferRequest) (application.TransferDetails, error) {
	return a.service.CreateDirectTransfer(a.ctx, request)
}

func (a *App) AcceptDirectTransfer(ticket string) (application.IncomingTransferDto, error) {
	return a.service.AcceptDirectTransfer(a.ctx, ticket)
}

func (a *App) ListIncomingTransfers() []application.IncomingTransferDto {
	return a.service.ListIncomingTransfers()
}

func (a *App) CancelDirectSession(sessionId string) error {
	return a.service.CancelDirectSession(sessionId)
}

func (a *App) GetRecommendedRelayURL() RelayRecommendationDto {
	endpoints, err := a.service.NetworkEndpoints()
	if err == nil && len(endpoints) > 0 {
		return RelayRecommendationDto{URL: "http://" + endpoints[0].IPAddress + ":8088", IsLocalLAN: true}
	}
	return RelayRecommendationDto{URL: "http://localhost:8088", IsLocalLAN: false}
}

func (a *App) GetTransferLimits() TransferLimitsDto {
	return TransferLimitsDto{
		LocalSingleFileLimitLabel:   "Без лимита приложения: ограничено диском, сетью и временем действия ссылки.",
		InternetSingleFileLimitGB:   50,
		InternetSingleFileLimitText: "До 50 ГБ на один файл при стандартном relay. Можно изменить флагом relay: -max-upload-gb.",
		MultiFileSupported:          false,
		MultiFileAdvice:             "Сейчас одна ссылка передаёт один файл. Для множества файлов упакуйте их в .zip или создайте несколько передач.",
	}
}

func (a *App) ListActiveTransfers() ([]application.TransferDetails, error) {
	return a.service.ListActive(a.ctx)
}

func (a *App) GetTransfer(transferId string) (application.TransferDetails, error) {
	return a.service.Get(a.ctx, transferId)
}

func (a *App) CancelTransfer(transferId string) error {
	if a.cloudpub != nil {
		_ = a.cloudpub.Stop(transferId)
	}
	return a.service.Cancel(a.ctx, transferId)
}

func (a *App) GetTransferQRCode(transferId string) (QrCodeDto, error) {
	png, err := a.service.QRCode(transferId)
	if err != nil {
		return QrCodeDto{}, err
	}
	return QrCodeDto{PNGBase64: base64.StdEncoding.EncodeToString(png)}, nil
}

func (a *App) SaveTransferQRCode(transferId string) (SaveResultDto, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save QR code",
		DefaultFilename: "droponce-" + transferId + ".png",
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "PNG image",
			Pattern:     "*.png",
		}},
	})
	if err != nil {
		return SaveResultDto{}, err
	}
	if path == "" {
		return SaveResultDto{}, errors.New("save cancelled")
	}
	png, err := a.service.QRCode(transferId)
	if err != nil {
		return SaveResultDto{}, err
	}
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return SaveResultDto{}, err
	}
	return SaveResultDto{Path: path}, nil
}

func (a *App) CopyTransferLink(transferId string) error {
	details, err := a.service.Get(a.ctx, transferId)
	if err != nil {
		return err
	}
	if details.ShareURL == "" {
		return errors.New("transfer link is no longer active")
	}
	wailsruntime.ClipboardSetText(a.ctx, details.ShareURL)
	return nil
}

func (a *App) RevealTransferSourceFile(transferId string) error {
	details, err := a.service.Get(a.ctx, transferId)
	if err != nil {
		return err
	}
	if details.SourcePath == "" {
		return errors.New("source path is not available")
	}
	wailsruntime.BrowserOpenURL(a.ctx, "file://"+filepath.Dir(details.SourcePath))
	return nil
}

func (a *App) ListTransferHistory(_ any) ([]application.TransferDetails, error) {
	return a.service.ListHistory(a.ctx)
}

func (a *App) DeleteHistoryItem(transferId string) error {
	return a.service.DeleteHistory(a.ctx, transferId)
}

func (a *App) ClearTransferHistory() error {
	return a.service.ClearHistory(a.ctx)
}

func (a *App) GetSettings() SettingsDto {
	return a.settings
}

func defaultSettings() SettingsDto {
	return SettingsDto{
		Language:              "ru",
		Theme:                 "system",
		DefaultRelayURL:       "",
		DefaultExpiryMinutes:  30,
		DefaultMaxDownloads:   1,
		DefaultCalculateSHA:   true,
		WarnTrustedLocalOnly:  true,
		MaxActiveTransfers:    10,
		ConfirmCloseWithLinks: true,
	}
}

func (a *App) UpdateSettings(request SettingsDto) (SettingsDto, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return SettingsDto{}, err
	}
	if err := a.service.SetSetting(a.ctx, "settings", string(body)); err != nil {
		return SettingsDto{}, err
	}
	a.settings = request
	return request, nil
}

func (a *App) GetDiagnostics() (DiagnosticsDto, error) {
	active, err := a.service.ListActive(a.ctx)
	if err != nil {
		return DiagnosticsDto{}, err
	}
	return DiagnosticsDto{
		Version:             "0.1.0",
		GoVersion:           runtime.Version(),
		WailsVersion:        "v2.12.0",
		SQLitePath:          a.dbPath,
		LogsPath:            a.logPath,
		ActiveServerCount:   a.service.ActiveServerCount(),
		ActiveTransferCount: len(active),
	}, nil
}

func appDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "DropOnce")
	return dir, os.MkdirAll(dir, 0o700)
}

func appCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "DropOnce", "temporary")
	return dir, os.MkdirAll(dir, 0o700)
}

func zipFiles(zipPath string, paths []string) error {
	out, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	archive := zip.NewWriter(out)
	defer archive.Close()

	usedNames := map[string]int{}
	for _, path := range paths {
		meta, err := filesystem.Inspect(path)
		if err != nil {
			return err
		}
		name := uniqueZipName(safeZipName(meta.Name), usedNames)
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.Modified = meta.ModifiedAt
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(meta.ResolvedPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(writer, file); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func safeZipName(name string) string {
	name = filepath.Base(strings.NewReplacer("\r", "", "\n", "", string(filepath.Separator), "-").Replace(name))
	if name == "." || name == "" {
		return "file"
	}
	return name
}

func uniqueZipName(name string, used map[string]int) string {
	count := used[name]
	used[name] = count + 1
	if count == 0 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return base + "-" + strconv.Itoa(count+1) + ext
}
