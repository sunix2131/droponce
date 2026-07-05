package transfer

import "time"

type Status string

const (
	StatusPreparing         Status = "preparing"
	StatusActive            Status = "active"
	StatusDownloading       Status = "downloading"
	StatusConsumed          Status = "consumed"
	StatusExpired           Status = "expired"
	StatusCancelled         Status = "cancelled"
	StatusFailed            Status = "failed"
	StatusEndedAfterRestart Status = "ended_after_restart"
)

type DownloadStatus string

const (
	DownloadReserved    DownloadStatus = "reserved"
	DownloadStreaming   DownloadStatus = "streaming"
	DownloadCompleted   DownloadStatus = "completed"
	DownloadInterrupted DownloadStatus = "interrupted"
	DownloadRejected    DownloadStatus = "rejected"
	DownloadFailed      DownloadStatus = "failed"
)

type Transfer struct {
	ID                 string    `json:"id"`
	Status             Status    `json:"status"`
	SourceFileName     string    `json:"sourceFileName"`
	SourcePath         string    `json:"sourcePath,omitempty"`
	SourceSizeBytes    int64     `json:"sourceSizeBytes"`
	SourceModifiedAt   time.Time `json:"sourceModifiedAt"`
	SourceSHA256       string    `json:"sourceSha256,omitempty"`
	BindIP             string    `json:"bindIp,omitempty"`
	Port               int       `json:"port,omitempty"`
	TokenHash          string    `json:"-"`
	MaxDownloads       int       `json:"maxDownloads"`
	CompletedDownloads int       `json:"completedDownloads"`
	ExpiresAt          time.Time `json:"expiresAt"`
	CreatedAt          time.Time `json:"createdAt"`
	ActivatedAt        time.Time `json:"activatedAt,omitempty"`
	CompletedAt        time.Time `json:"completedAt,omitempty"`
	CancelledAt        time.Time `json:"cancelledAt,omitempty"`
	StoppedAt          time.Time `json:"stoppedAt,omitempty"`
	LastErrorCode      string    `json:"lastErrorCode,omitempty"`
	LastErrorMessage   string    `json:"lastErrorMessage,omitempty"`
}

func (t Transfer) RemainingDownloads() int {
	left := t.MaxDownloads - t.CompletedDownloads
	if left < 0 {
		return 0
	}
	return left
}

func (t Transfer) IsRuntimeActive(now time.Time) bool {
	return (t.Status == StatusActive || t.Status == StatusDownloading) &&
		now.Before(t.ExpiresAt) &&
		t.CompletedDownloads < t.MaxDownloads
}

type DownloadAttempt struct {
	ID           string
	TransferID   string
	Status       DownloadStatus
	StartedAt    time.Time
	CompletedAt  time.Time
	BytesSent    int64
	ErrorCode    string
	ErrorMessage string
}

type NetworkEndpoint struct {
	InterfaceName string `json:"interfaceName"`
	IPAddress     string `json:"ipAddress"`
	DisplayName   string `json:"displayName"`
	IsPrivateIPv4 bool   `json:"isPrivateIpv4"`
	IsUp          bool   `json:"isUp"`
}
