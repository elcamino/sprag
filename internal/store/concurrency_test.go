// Zener - a tiny anonymous file dropbox.
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
	"path/filepath"
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
		_ = conn.Close()
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
