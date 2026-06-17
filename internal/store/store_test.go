// Zener - a post-quantum-safe end-to-end encrypted file dropbox.
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
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tob/zener/internal/store"
)

func TestSQLiteStoreCreatesPagesAndAggregatesUploads(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "zener.db"))
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

	if _, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/abc/file-1/report.pdf",
		OriginalName: "report.pdf",
		SizeBytes:    12,
		ContentType:  "application/pdf",
		UploaderIP:   "203.0.113.7",
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
	if !files[0].UploadedAt.After(time.Time{}) {
		t.Fatal("expected uploaded_at to be populated")
	}
}

func TestSQLiteStoreRejectsDuplicateSlugs(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "zener.db"))
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

func ptrInt64(v int64) *int64 {
	return &v
}
