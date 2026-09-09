package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReplacementConnectionsKeepForeignKeys(t *testing.T) {
	db, _, err := Open(context.Background(), filepath.Join(t.TempDir(), "data # with spaces"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxIdleConns(0)
	for i := 0; i < 3; i++ {
		var enabled, timeout int
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatal(err)
		}
		if enabled != 1 {
			t.Fatalf("foreign keys disabled on replacement connection: %d", enabled)
		}
		if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatal(err)
		}
		if timeout != 5000 {
			t.Fatalf("busy timeout lost: %d", timeout)
		}
	}
}
