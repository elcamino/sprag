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
