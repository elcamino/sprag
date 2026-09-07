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

package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elcamino/sprag/internal/blob"
	httpapi "github.com/elcamino/sprag/internal/http"
	"github.com/elcamino/sprag/internal/store"
)

func lifecycleHandler(t *testing.T, objects blob.Store) (http.Handler, *store.SQLite) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h, err := httpapi.New(httpapi.Dependencies{Store: db, BlobStore: objects,
		Config: httpapi.Config{BaseURL: "https://sprag.example.test", SessionSecret: []byte("12345678901234567890123456789012"), AdminUsername: "admin", AdminPassword: "correct-password", MaxFileSize: 1024},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return h, db
}

type deletionBlobStore struct {
	memoryBlobStore
	beforeDelete func()
	calls        int
	failCall     int
}

func (b *deletionBlobStore) Delete(ctx context.Context, key string) error {
	b.calls++
	if b.beforeDelete != nil {
		b.beforeDelete()
	}
	if b.calls == b.failCall {
		return errors.New("injected storage failure")
	}
	return b.memoryBlobStore.Delete(ctx, key)
}

func TestBulkDeletionBlocksConcurrentSealAndReopen(t *testing.T) {
	b := &deletionBlobStore{memoryBlobStore: memoryBlobStore{objects: map[string][]byte{}}}
	h, db := lifecycleHandler(t, b)
	session := loginAdmin(t, h)
	slug := createPageSlug(t, h, session, map[string]any{"title": "Deletion wins"})
	up := performMultipart(t, h, "/api/u/"+slug, "file", "evidence.txt", []byte("evidence"), nil)
	if up.Code != http.StatusCreated {
		t.Fatal(up.Body.String())
	}
	b.beforeDelete = func() {
		seal := performJSON(t, h, http.MethodPost, "/api/admin/pages/1/seal", map[string]any{}, session, csrfHeader())
		if seal.Code != http.StatusConflict || !strings.Contains(seal.Body.String(), "page_deleting") {
			t.Errorf("seal during deletion: %d %s", seal.Code, seal.Body.String())
		}
		update := performJSON(t, h, http.MethodPatch, "/api/admin/pages/1", map[string]bool{"is_active": true}, session, csrfHeader())
		if update.Code != http.StatusConflict {
			t.Errorf("reopen during deletion: %d", update.Code)
		}
		remove := perform(t, h, http.MethodDelete, "/api/admin/pages/1", nil, session, csrfHeader())
		if remove.Code != http.StatusConflict {
			t.Errorf("metadata-only deletion during bulk deletion: %d", remove.Code)
		}
		public := perform(t, h, http.MethodGet, "/api/u/"+slug, nil, nil, nil)
		if public.Code != http.StatusNotFound {
			t.Errorf("intake remains open: %d", public.Code)
		}
	}
	r := perform(t, h, http.MethodDelete, "/api/admin/pages/1?files=1", nil, session, csrfHeader())
	if r.Code != http.StatusNoContent || len(b.objects) != 0 {
		t.Fatalf("deletion failed: %d %s", r.Code, r.Body.String())
	}
	if _, err := db.GetPage(context.Background(), 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("page survived deletion: %v", err)
	}
}

func TestSealPreventsBulkDeletionBeforeStorageIsTouched(t *testing.T) {
	b := &deletionBlobStore{memoryBlobStore: memoryBlobStore{objects: map[string][]byte{}}}
	h, _ := lifecycleHandler(t, b)
	session := loginAdmin(t, h)
	slug := createPageSlug(t, h, session, map[string]any{"title": "Seal wins"})
	up := performMultipart(t, h, "/api/u/"+slug, "file", "evidence.txt", []byte("evidence"), nil)
	if up.Code != http.StatusCreated {
		t.Fatal(up.Body.String())
	}
	seal := performJSON(t, h, http.MethodPost, "/api/admin/pages/1/seal", map[string]any{}, session, csrfHeader())
	if seal.Code != http.StatusOK {
		t.Fatal(seal.Body.String())
	}
	r := perform(t, h, http.MethodDelete, "/api/admin/pages/1?files=1", nil, session, csrfHeader())
	if r.Code != http.StatusConflict || b.calls != 0 || len(b.objects) != 1 {
		t.Fatalf("sealed evidence touched: status=%d calls=%d objects=%d", r.Code, b.calls, len(b.objects))
	}
}

func TestPartialBulkDeletionRetainsAuditAndCanBeRetried(t *testing.T) {
	b := &deletionBlobStore{memoryBlobStore: memoryBlobStore{objects: map[string][]byte{}}, failCall: 2}
	h, db := lifecycleHandler(t, b)
	session := loginAdmin(t, h)
	slug := createPageSlug(t, h, session, map[string]any{"title": "Retry deletion"})
	for _, name := range []string{"one.txt", "two.txt"} {
		up := performMultipart(t, h, "/api/u/"+slug, "file", name, []byte("evidence"), nil)
		if up.Code != http.StatusCreated {
			t.Fatal(up.Body.String())
		}
	}
	r := perform(t, h, http.MethodDelete, "/api/admin/pages/1?files=1", nil, session, csrfHeader())
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("expected storage failure, got %d", r.Code)
	}
	page, err := db.GetPage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !page.DeletionPending || page.IsActive || page.UploadCount != 1 || len(b.objects) != 1 {
		t.Fatalf("invalid recoverable state: %#v, objects=%d", page, len(b.objects))
	}
	events, err := db.ListCustodyEvents(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	deleted := 0
	for _, event := range events {
		if event.EventType == "file.deleted" {
			deleted++
			if !strings.Contains(event.Detail, sha512Hex([]byte("evidence"))) {
				t.Fatal("deletion event lost object hash")
			}
		}
	}
	if deleted != 1 {
		t.Fatalf("completed deletion events=%d, want 1", deleted)
	}
	b.failCall = 0
	r = perform(t, h, http.MethodDelete, "/api/admin/pages/1?files=1", nil, session, csrfHeader())
	if r.Code != http.StatusNoContent || len(b.objects) != 0 {
		t.Fatalf("retry failed: %d %s", r.Code, r.Body.String())
	}
}

func TestUploadRejectsPageClosedWhileStreaming(t *testing.T) {
	for _, action := range []string{"seal", "deactivate", "expire", "delete", "bulk-delete"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			objects := &memoryBlobStore{objects: map[string][]byte{}}
			hooked := &sabotagingBlobStore{inner: objects}
			h, db := lifecycleHandler(t, hooked)
			session := loginAdmin(t, h)
			slug := createPageSlug(t, h, session, map[string]any{"title": "Closing during upload"})
			hooked.afterUpload = func() {
				var err error
				switch action {
				case "seal":
					_, err = db.SealPage(ctx, 1)
				case "deactivate":
					active := false
					_, err = db.UpdatePage(ctx, 1, store.PageUpdate{IsActive: &active})
				case "expire":
					past := time.Now().Add(-time.Minute)
					_, err = db.UpdatePage(ctx, 1, store.PageUpdate{ExpiresAt: store.NullableTime{Set: true, Value: &past}})
				case "delete":
					err = db.DeletePage(ctx, 1)
				case "bulk-delete":
					err = db.BeginPageDeletion(ctx, 1)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			r := performMultipart(t, h, "/api/u/"+slug, "file", "late.txt", []byte("late evidence"), nil)
			if r.Code != http.StatusNotFound || !strings.Contains(r.Body.String(), "page_closed") {
				t.Fatalf("late upload: %d %s", r.Code, r.Body.String())
			}
			if len(objects.objects) != 0 {
				t.Fatal("rejected upload left an orphaned blob")
			}
			files, err := db.ListUploads(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 {
				t.Fatal("closed page accepted upload metadata")
			}
			events, err := db.ListCustodyEvents(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.EventType == "upload.accepted" {
					t.Fatal("closed page recorded acceptance")
				}
			}
		})
	}
}

func TestUploadAcceptsPageBeforeExpiry(t *testing.T) {
	objects := &memoryBlobStore{objects: map[string][]byte{}}
	h, _ := lifecycleHandler(t, objects)
	session := loginAdmin(t, h)
	slug := createPageSlug(t, h, session, map[string]any{"title": "Still open", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
	r := performMultipart(t, h, "/api/u/"+slug, "file", "on-time.txt", []byte("evidence"), nil)
	if r.Code != http.StatusCreated {
		t.Fatalf("open page rejected upload: %d %s", r.Code, r.Body.String())
	}
}

func TestHandledSubmissionRejectsAdditionalFiles(t *testing.T) {
	for _, status := range []string{"reviewed", "rejected", "downloaded"} {
		t.Run(status, func(t *testing.T) {
			h, objects := newTestHandler(t)
			session := loginAdmin(t, h)
			slug := createPageSlug(t, h, session, map[string]any{"title": "Completed submission"})
			fields := map[string]string{"submission_id": "submission-reused"}
			for _, name := range []string{"first.txt", "second.txt"} {
				r := performMultipartFields(t, h, "/api/u/"+slug, fields, "file", name, []byte("evidence"), nil)
				if r.Code != http.StatusCreated {
					t.Fatal(r.Body.String())
				}
			}
			update := performJSON(t, h, http.MethodPatch, "/api/admin/pages/1/submissions/submission-reused/receipt", map[string]string{"status": status}, session, csrfHeader())
			if update.Code != http.StatusOK {
				t.Fatal(update.Body.String())
			}
			rejected := performMultipartFields(t, h, "/api/u/"+slug, fields, "file", "later.txt", []byte("unreviewed"), nil)
			if rejected.Code != http.StatusConflict || !strings.Contains(rejected.Body.String(), "submission_closed") {
				t.Fatalf("handled submission accepted append: %d %s", rejected.Code, rejected.Body.String())
			}
			if len(objects.objects) != 2 {
				t.Fatalf("append left a stored object: %d", len(objects.objects))
			}
			files := perform(t, h, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
			var listed []store.Upload
			decodeJSON(t, files.Body.Bytes(), &listed)
			if len(listed) != 2 || listed[0].ReceiptStatus != status {
				t.Fatalf("submission changed: %#v", listed)
			}
			fresh := performMultipartFields(t, h, "/api/u/"+slug, map[string]string{"submission_id": "new-submission-id"}, "file", "new.txt", []byte("unreviewed"), nil)
			if fresh.Code != http.StatusCreated {
				t.Fatalf("new submission rejected: %d", fresh.Code)
			}
		})
	}
}
