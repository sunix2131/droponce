package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"droponce/internal/domain/transfer"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertTransfer(ctx context.Context, t transfer.Transfer) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO transfers (
id,status,source_file_name,source_path,source_size_bytes,source_modified_at,source_sha256,bind_ip,port,token_hash,
max_downloads,completed_downloads,expires_at,created_at,activated_at,completed_at,cancelled_at,stopped_at,last_error_code,last_error_message
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
status=excluded.status, source_file_name=excluded.source_file_name, source_path=excluded.source_path,
source_size_bytes=excluded.source_size_bytes, source_modified_at=excluded.source_modified_at, source_sha256=excluded.source_sha256,
bind_ip=excluded.bind_ip, port=excluded.port, token_hash=excluded.token_hash, max_downloads=excluded.max_downloads,
completed_downloads=excluded.completed_downloads, expires_at=excluded.expires_at, activated_at=excluded.activated_at,
completed_at=excluded.completed_at, cancelled_at=excluded.cancelled_at, stopped_at=excluded.stopped_at,
last_error_code=excluded.last_error_code, last_error_message=excluded.last_error_message`,
		t.ID, string(t.Status), t.SourceFileName, nullable(t.SourcePath), t.SourceSizeBytes, formatTime(t.SourceModifiedAt), nullable(t.SourceSHA256),
		nullable(t.BindIP), t.Port, t.TokenHash, t.MaxDownloads, t.CompletedDownloads, formatTime(t.ExpiresAt), formatTime(t.CreatedAt),
		nullableTime(t.ActivatedAt), nullableTime(t.CompletedAt), nullableTime(t.CancelledAt), nullableTime(t.StoppedAt), nullable(t.LastErrorCode), nullable(t.LastErrorMessage))
	return err
}

func (r *Repository) GetTransfer(ctx context.Context, id string) (transfer.Transfer, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,status,source_file_name,COALESCE(source_path,''),source_size_bytes,source_modified_at,
COALESCE(source_sha256,''),COALESCE(bind_ip,''),COALESCE(port,0),token_hash,max_downloads,completed_downloads,expires_at,created_at,
COALESCE(activated_at,''),COALESCE(completed_at,''),COALESCE(cancelled_at,''),COALESCE(stopped_at,''),COALESCE(last_error_code,''),COALESCE(last_error_message,'')
FROM transfers WHERE id=?`, id)
	return scanTransfer(row)
}

func (r *Repository) GetTransferByHash(ctx context.Context, hash string) (transfer.Transfer, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,status,source_file_name,COALESCE(source_path,''),source_size_bytes,source_modified_at,
COALESCE(source_sha256,''),COALESCE(bind_ip,''),COALESCE(port,0),token_hash,max_downloads,completed_downloads,expires_at,created_at,
COALESCE(activated_at,''),COALESCE(completed_at,''),COALESCE(cancelled_at,''),COALESCE(stopped_at,''),COALESCE(last_error_code,''),COALESCE(last_error_message,'')
FROM transfers WHERE token_hash=?`, hash)
	return scanTransfer(row)
}

func (r *Repository) ListTransfers(ctx context.Context, activeOnly bool) ([]transfer.Transfer, error) {
	query := `SELECT id,status,source_file_name,COALESCE(source_path,''),source_size_bytes,source_modified_at,
COALESCE(source_sha256,''),COALESCE(bind_ip,''),COALESCE(port,0),token_hash,max_downloads,completed_downloads,expires_at,created_at,
COALESCE(activated_at,''),COALESCE(completed_at,''),COALESCE(cancelled_at,''),COALESCE(stopped_at,''),COALESCE(last_error_code,''),COALESCE(last_error_message,'') FROM transfers`
	if activeOnly {
		query += ` WHERE status IN ('active','downloading','preparing')`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []transfer.Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) MarkRestarted(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE transfers SET status='ended_after_restart', stopped_at=?, source_path=NULL
WHERE status IN ('preparing','active','downloading')`, formatTime(now))
	return err
}

func (r *Repository) DeleteTransfer(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM transfers WHERE id=?", id)
	return err
}

func (r *Repository) ClearHistory(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM transfers WHERE status NOT IN ('active','downloading','preparing')")
	return err
}

func (r *Repository) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO settings (key,value) VALUES (?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (r *Repository) AddAttempt(ctx context.Context, a transfer.DownloadAttempt) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO download_attempts (id,transfer_id,status,started_at,bytes_sent,error_code,error_message)
VALUES (?,?,?,?,?,?,?)`, a.ID, a.TransferID, string(a.Status), formatTime(a.StartedAt), a.BytesSent, nullable(a.ErrorCode), nullable(a.ErrorMessage))
	return err
}

func (r *Repository) FinishAttempt(ctx context.Context, id string, status transfer.DownloadStatus, bytes int64, code, msg string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE download_attempts SET status=?, completed_at=?, bytes_sent=?, error_code=?, error_message=? WHERE id=?`,
		string(status), formatTime(at), bytes, nullable(code), nullable(msg), id)
	return err
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTransfer(row rowScanner) (transfer.Transfer, error) {
	var t transfer.Transfer
	var status, modified, expires, created, activated, completed, cancelled, stopped string
	err := row.Scan(&t.ID, &status, &t.SourceFileName, &t.SourcePath, &t.SourceSizeBytes, &modified, &t.SourceSHA256,
		&t.BindIP, &t.Port, &t.TokenHash, &t.MaxDownloads, &t.CompletedDownloads, &expires, &created, &activated,
		&completed, &cancelled, &stopped, &t.LastErrorCode, &t.LastErrorMessage)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return transfer.Transfer{}, err
		}
		return transfer.Transfer{}, err
	}
	t.Status = transfer.Status(status)
	t.SourceModifiedAt = parseTime(modified)
	t.ExpiresAt = parseTime(expires)
	t.CreatedAt = parseTime(created)
	t.ActivatedAt = parseTime(activated)
	t.CompletedAt = parseTime(completed)
	t.CancelledAt = parseTime(cancelled)
	t.StoppedAt = parseTime(stopped)
	return t, nil
}

func parseTime(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}
