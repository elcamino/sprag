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
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/elcamino/sprag/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestZipReturnsErrorWhenObjectMissing covers the common failure mode: a row
// exists in the database but the object is gone from storage. The handler must
// surface a clean error status instead of a 200 with a truncated archive.
func TestZipReturnsErrorWhenObjectMissing(t *testing.T) {
	handler, blobs := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Broken"})

	up := performMultipart(t, handler, "/api/u/"+slug, "file", "doc.txt", []byte("data"), nil)
	if up.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", up.Code, up.Body.String())
	}
	// The object vanishes from storage while the DB row remains.
	for k := range blobs.objects {
		delete(blobs.objects, k)
	}

	zipResp := perform(t, handler, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	if zipResp.Code != http.StatusInternalServerError {
		t.Fatalf("zip with missing object status = %d (body len %d), want 500", zipResp.Code, zipResp.Body.Len())
	}
}

// TestZipAbortsConnectionOnMidStreamFailure covers a failure that only shows up
// after streaming has begun (the object passes the pre-flight check but its body
// errors mid-copy). The handler cannot change the status code at that point, so
// it must abort the connection rather than finalize a truncated-but-valid zip.
func TestZipAbortsConnectionOnMidStreamFailure(t *testing.T) {
	handler := newHandlerWith(t, faultyBlobStore{}, nil)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Faulty"})

	up := performMultipart(t, handler, "/api/u/"+slug, "file", "doc.txt", []byte("data"), nil)
	if up.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", up.Code, up.Body.String())
	}

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Fatalf("expected panic(http.ErrAbortHandler), got %v", rec)
		}
	}()
	perform(t, handler, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	t.Fatal("expected zip handler to abort the connection")
}

// faultyBlobStore accepts uploads and lets Download succeed (so the pre-flight
// existence check passes) but returns a body that errors on the first read,
// simulating a mid-stream storage failure.
type faultyBlobStore struct{}

func (faultyBlobStore) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, _ = io.Copy(io.Discard, body)
	return nil
}

func (faultyBlobStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(errReader{}), nil
}

func (faultyBlobStore) Delete(ctx context.Context, key string) error {
	return nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated storage read failure")
}

func requireZipAbort(t *testing.T, action func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Errorf("expected aborted ZIP transfer, got %v", r)
		}
	}()
	action()
	t.Error("corrupt ZIP transfer completed normally")
}

func TestZipRejectsStoredObjectCorruption(t *testing.T) {
	for _, content := range []string{"cut", "complete evidence with extra bytes", "COMPLETE EVIDENCE"} {
		t.Run(content, func(t *testing.T) {
			objects := &memoryBlobStore{objects: map[string][]byte{}}
			h, db := lifecycleHandler(t, objects)
			session := loginAdmin(t, h)
			slug := createPageSlug(t, h, session, map[string]any{"title": "Integrity"})
			up := performMultipart(t, h, "/api/u/"+slug, "file", "evidence.txt", []byte("complete evidence"), nil)
			if up.Code != http.StatusCreated {
				t.Fatal(up.Body.String())
			}
			for key := range objects.objects {
				objects.objects[key] = []byte(content)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/admin/pages/1/zip", nil)
			for _, cookie := range session {
				request.AddCookie(cookie)
			}
			requireZipAbort(t, func() { h.ServeHTTP(recorder, request) })
			if _, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len())); err == nil {
				t.Fatal("corrupt content produced a valid ZIP central directory")
			}
			events, err := db.ListCustodyEvents(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.EventType == "page.exported" {
					t.Fatal("corrupt ZIP recorded successful export")
				}
			}
		})
	}
}

func TestZipSupportsLegacyObjectsWithoutHashes(t *testing.T) {
	ctx := context.Background()
	objects := &memoryBlobStore{objects: map[string][]byte{"legacy": []byte("legacy bytes")}}
	h, db := lifecycleHandler(t, objects)
	page, err := db.CreatePage(ctx, store.PageCreate{Slug: "legacy", Title: "Legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUpload(ctx, store.UploadCreate{PageID: page.ID, S3Key: "legacy", OriginalName: "legacy.txt", SizeBytes: 12}); err != nil {
		t.Fatal(err)
	}
	session := loginAdmin(t, h)
	r := perform(t, h, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	zr, err := zip.NewReader(bytes.NewReader(r.Body.Bytes()), int64(r.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil || string(content) != "legacy bytes" {
		t.Fatalf("legacy content=%q error=%v", content, err)
	}
}

type failingZipWriter struct{ *httptest.ResponseRecorder }

func (failingZipWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected finalization error")
}

func TestZipFinalizationFailureDoesNotRecordExport(t *testing.T) {
	objects := &memoryBlobStore{objects: map[string][]byte{}}
	h, db := lifecycleHandler(t, objects)
	session := loginAdmin(t, h)
	slug := createPageSlug(t, h, session, map[string]any{"title": "Finalization"})
	up := performMultipart(t, h, "/api/u/"+slug, "file", "small.txt", []byte("small"), nil)
	if up.Code != http.StatusCreated {
		t.Fatal(up.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/pages/1/zip", nil)
	for _, cookie := range session {
		request.AddCookie(cookie)
	}
	requireZipAbort(t, func() { h.ServeHTTP(failingZipWriter{httptest.NewRecorder()}, request) })
	events, err := db.ListCustodyEvents(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.EventType, "exported") {
			t.Fatal("failed finalization recorded export")
		}
	}
}
