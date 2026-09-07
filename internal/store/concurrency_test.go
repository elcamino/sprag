// Sprag - a post-quantum-safe end-to-end encrypted file dropbox.
// Copyright (C) 2026 Tobias von Dewitz <tobias@vondewitz.org>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestOpenEnablesWALAndPerConnectionBusyTimeout(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "z.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	// busy_timeout is a per-connection setting, so it must be applied to every
	// pooled connection (via the DSN), not just the one that ran migrations.
	s.db.SetMaxOpenConns(4)
	for i := 0; i < 4; i++ {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		var mode string
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("journal_mode on conn %d: %v", i, err)
		}
		if mode != "wal" {
			t.Fatalf("conn %d journal_mode = %q, want wal", i, mode)
		}
		var timeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatalf("busy_timeout on conn %d: %v", i, err)
		}
		if timeout < 1000 {
			t.Fatalf("conn %d busy_timeout = %d, want >= 1000", i, timeout)
		}
		defer conn.Close()
	}
}

// Hold every connection until the test ends so each check covers a distinct
// connection, including connections that did not execute migrations.
func TestOpenEnforcesForeignKeysOnEveryConnection(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "z.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	s.db.SetMaxOpenConns(4)
	for i := 0; i < 4; i++ {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		defer conn.Close()
		var enabled int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatalf("foreign_keys on conn %d: %v", i, err)
		}
		if enabled != 1 {
			t.Fatalf("conn %d foreign_keys = %d, want 1", i, enabled)
		}
	}
}

func TestDeletePageCascadesOnNewConnection(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "cascade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.CreatePage(ctx, PageCreate{Slug: "original", Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUpload(ctx, UploadCreate{PageID: page.ID, S3Key: "object", OriginalName: "file", SizeBytes: 1}); err != nil {
		t.Fatal(err)
	}
	first, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := s.DeletePage(ctx, page.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.CreatePage(ctx, PageCreate{Slug: "replacement", Title: "Replacement"})
	if err != nil {
		t.Fatal(err)
	}
	if next.UploadCount != 0 {
		t.Fatalf("new page inherited %d uploads", next.UploadCount)
	}
	for _, table := range []string{"uploads", "submission_envelopes", "custody_events"} {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d orphaned records", table, count)
		}
	}
}

func TestOpenRejectsExistingForeignKeyViolations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "invalid.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(ctx, `INSERT INTO uploads (page_id, s3_key, original_name, size_bytes) VALUES (999, 'orphan', 'orphan', 1)`); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err == nil {
		reopened.Close()
		t.Fatal("opened a database with orphaned uploads")
	}
	if !strings.Contains(err.Error(), "foreign key violation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConcurrentUploadsDoNotErrorUnderContention(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "z.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	page, err := s.CreatePage(ctx, PageCreate{Slug: "concurrencyslug01", Title: "Load"})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.CreateUpload(ctx, UploadCreate{
				PageID:       page.ID,
				S3Key:        "k",
				OriginalName: "f",
				SizeBytes:    1,
			}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent CreateUpload failed (likely SQLITE_BUSY): %v", err)
	}
}

func TestOpenInMemoryStillWorks(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer s.Close()
	if _, err := s.CreatePage(ctx, PageCreate{Slug: "memoryslug000001", Title: "T"}); err != nil {
		t.Fatalf("CreatePage on in-memory store failed: %v", err)
	}
}
