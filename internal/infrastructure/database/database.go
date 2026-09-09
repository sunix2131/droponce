package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(ctx context.Context, dir string) (*sql.DB, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, "droponce.db")
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	// PRAGMAs are applied by the driver to every connection, including replacements.
	dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Hour)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000; PRAGMA synchronous = NORMAL;"); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		_ = db.Close()
		return nil, "", err
	}
	for _, file := range files {
		body, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			_ = db.Close()
			return nil, "", err
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			_ = db.Close()
			return nil, "", fmt.Errorf("%s: %w", file.Name(), err)
		}
	}
	return db, path, nil
}
