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
		ObjectSHA512: "hash-one",
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
		ObjectSHA512: "hash-two",
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
		if file.ObjectHashAlgorithm != "SHA-512" || file.ObjectSHA512 == "" {
			t.Fatalf("file %q missing stored object hash: algorithm=%q hash=%q", file.OriginalName, file.ObjectHashAlgorithm, file.ObjectSHA512)
		}
	}
	if !files[0].UploadedAt.After(time.Time{}) {
		t.Fatal("expected uploaded_at to be populated")
	}
}

func TestSQLiteStoreCreatesReceiptForSubmissionEnvelope(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	page, err := db.CreatePage(ctx, store.PageCreate{
		Slug:  "receipt-page-1",
		Title: "Receipt intake",
	})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}

	submissionID := "33333333-3333-4333-8333-333333333333"
	first, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/receipt/file-1/one.pdf",
		OriginalName: "one.pdf",
		SizeBytes:    12,
		SubmissionID: submissionID,
	})
	if err != nil {
		t.Fatalf("CreateUpload first failed: %v", err)
	}
	second, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/receipt/file-2/two.pdf",
		OriginalName: "two.pdf",
		SizeBytes:    34,
		SubmissionID: submissionID,
	})
	if err != nil {
		t.Fatalf("CreateUpload second failed: %v", err)
	}
	if first.ReceiptToken == "" {
		t.Fatal("first upload missing receipt token")
	}
	if first.ReceiptToken != second.ReceiptToken {
		t.Fatalf("uploads in one submission got different receipt tokens: %q vs %q", first.ReceiptToken, second.ReceiptToken)
	}
	if first.ReceiptStatus != "received" || second.ReceiptStatus != "received" {
		t.Fatalf("receipt statuses = %q/%q, want received", first.ReceiptStatus, second.ReceiptStatus)
	}

	receipt, err := db.GetReceipt(ctx, first.ReceiptToken)
	if err != nil {
		t.Fatalf("GetReceipt failed: %v", err)
	}
	if receipt.Status != "received" {
		t.Fatalf("receipt status = %q, want received", receipt.Status)
	}
	if receipt.FileCount != 2 || receipt.TotalBytes != 46 {
		t.Fatalf("receipt aggregate = %d files/%d bytes, want 2 files/46 bytes", receipt.FileCount, receipt.TotalBytes)
	}
	if receipt.SubmittedAt.IsZero() || receipt.UpdatedAt.IsZero() {
		t.Fatalf("receipt times were not populated: submitted=%v updated=%v", receipt.SubmittedAt, receipt.UpdatedAt)
	}
}

func TestSQLiteStoreUpdatesReceiptStatus(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	page, err := db.CreatePage(ctx, store.PageCreate{
		Slug:  "receipt-status-1",
		Title: "Receipt status",
	})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}
	created, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/receipt/file/report.pdf",
		OriginalName: "report.pdf",
		SizeBytes:    12,
		SubmissionID: "44444444-4444-4444-8444-444444444444",
	})
	if err != nil {
		t.Fatalf("CreateUpload failed: %v", err)
	}

	updated, err := db.UpdateReceiptStatus(ctx, page.ID, created.SubmissionID, "reviewed")
	if err != nil {
		t.Fatalf("UpdateReceiptStatus failed: %v", err)
	}
	if updated.ReceiptStatus != "reviewed" {
		t.Fatalf("updated receipt status = %q, want reviewed", updated.ReceiptStatus)
	}
	receipt, err := db.GetReceipt(ctx, created.ReceiptToken)
	if err != nil {
		t.Fatalf("GetReceipt failed: %v", err)
	}
	if receipt.Status != "reviewed" {
		t.Fatalf("public receipt status = %q, want reviewed", receipt.Status)
	}

	if _, err := db.UpdateReceiptStatus(ctx, page.ID, created.SubmissionID, "chatting"); err == nil {
		t.Fatal("expected invalid receipt status to fail")
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
	if files[0].ReceiptToken == "" {
		t.Fatal("legacy upload missing backfilled receipt token")
	}
	receipt, err := db.GetReceipt(ctx, files[0].ReceiptToken)
	if err != nil {
		t.Fatalf("GetReceipt for backfilled legacy token failed: %v", err)
	}
	if receipt.Status != "received" || receipt.FileCount != 1 || receipt.TotalBytes != 12 {
		t.Fatalf("legacy receipt = %#v, want received/1 file/12 bytes", receipt)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
