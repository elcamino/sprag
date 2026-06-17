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

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/elcamino/sprag/internal/store"
)

func TestSQLiteStoreCreatesPagesAndAggregatesUploads(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	page, err := db.CreatePage(ctx, store.PageCreate{
		Slug:        "abc123abc123abc1",
		Title:       "Client drop",
		Description: "PDFs only",
		AllowedExt:  "pdf",
		MaxFileSize: ptrInt64(1024),
	})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}

	submissionID := "22222222-2222-4222-8222-222222222222"
	if _, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/abc/file-1/report.pdf",
		OriginalName: "report.pdf",
		SizeBytes:    12,
		ContentType:  "application/pdf",
		UploaderIP:   "203.0.113.7",
		SubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("CreateUpload failed: %v", err)
	}
	if _, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/abc/file-2/scan.pdf",
		OriginalName: "scan.pdf",
		SizeBytes:    34,
		ContentType:  "application/pdf",
		UploaderIP:   "203.0.113.7",
		SubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("CreateUpload failed: %v", err)
	}

	pages, err := db.ListPages(ctx)
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected one page, got %d", len(pages))
	}
	if pages[0].UploadCount != 2 || pages[0].TotalBytes != 46 {
		t.Fatalf("unexpected upload aggregate: count=%d bytes=%d", pages[0].UploadCount, pages[0].TotalBytes)
	}

	files, err := db.ListUploads(ctx, page.ID)
	if err != nil {
		t.Fatalf("ListUploads failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two files, got %d", len(files))
	}
	for _, file := range files {
		if file.SubmissionID != submissionID {
			t.Fatalf("file %q submission id = %q, want %q", file.OriginalName, file.SubmissionID, submissionID)
		}
		if file.SubmissionUploadedAt == nil {
			t.Fatalf("file %q missing submission upload time", file.OriginalName)
		}
	}
	if !files[0].UploadedAt.After(time.Time{}) {
		t.Fatal("expected uploaded_at to be populated")
	}
}

func TestSQLiteStoreRejectsDuplicateSlugs(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	create := store.PageCreate{Slug: "same-slug-same-1", Title: "One"}
	if _, err := db.CreatePage(ctx, create); err != nil {
		t.Fatalf("first CreatePage failed: %v", err)
	}
	_, err = db.CreatePage(ctx, create)
	if !errors.Is(err, store.ErrDuplicateSlug) {
		t.Fatalf("expected ErrDuplicateSlug, got %v", err)
	}
}

func TestSQLiteStoreMigratesLegacyUploadsIntoSubmissionEnvelopes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sprag.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw sqlite failed: %v", err)
	}
	_, err = raw.Exec(`
CREATE TABLE pages (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT,
  pin_hash TEXT,
  max_file_size INTEGER,
  allowed_ext TEXT,
  expires_at TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  e2e_enabled INTEGER NOT NULL DEFAULT 0,
  e2e_algorithm TEXT,
  e2e_public_key TEXT,
  e2e_public_key_fingerprint TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE uploads (
  id INTEGER PRIMARY KEY,
  page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  s3_key TEXT NOT NULL,
  original_name TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  content_type TEXT,
  uploader_ip TEXT,
  encryption_mode TEXT,
  encryption_algorithm TEXT,
  encryption_envelope TEXT,
  uploaded_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO pages (id, slug, title) VALUES (1, 'legacy-legacy-1', 'Legacy');
INSERT INTO uploads (id, page_id, s3_key, original_name, size_bytes, uploader_ip, uploaded_at)
VALUES (9, 1, 'pages/legacy/file/report.pdf', 'report.pdf', 12, '203.0.113.9', '2026-06-17T10:00:00Z');
`)
	if closeErr := raw.Close(); closeErr != nil {
		t.Fatalf("close raw sqlite failed: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("seed legacy schema failed: %v", err)
	}

	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	files, err := db.ListUploads(ctx, 1)
	if err != nil {
		t.Fatalf("ListUploads failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0].SubmissionID != "legacy-9" {
		t.Fatalf("submission id = %q, want legacy-9", files[0].SubmissionID)
	}
	if files[0].SubmissionUploadedAt == nil || !files[0].SubmissionUploadedAt.Equal(time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected submission upload time: %v", files[0].SubmissionUploadedAt)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
