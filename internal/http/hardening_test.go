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
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpapi "github.com/elcamino/sprag/internal/http"
	"github.com/elcamino/sprag/internal/store"
)

// An uploader never needs more than a handful of form fields (submission id,
// E2E envelope). Without a cap on the number of non-file parts, a client can
// stream an unbounded sequence of just-under-64KB fields and burn CPU and
// memory before the handler ever rejects the request.
func TestUploadRejectsExcessiveMultipartFields(t *testing.T) {
	handler, blobs := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Field flood page",
	})

	fields := map[string]string{}
	for i := 0; i < 40; i++ {
		fields[fmt.Sprintf("junk_%02d", i)] = "x"
	}
	resp := performMultipartFields(t, handler, "/api/u/"+slug, fields, "file", "note.txt", []byte("hello"), nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "too_many_fields") {
		t.Fatalf("body = %s, want too_many_fields error code", resp.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("blob was written despite rejected request: %d objects", len(blobs.objects))
	}
}

// The request body must be bounded before multipart parsing begins. Skipped
// parts (wrong field name) are drained and discarded, so without a body cap a
// client can stream gigabytes of junk parts the server reads in full even
// though the file-part limit is never reached.
func TestUploadRejectsBodyExceedingSizeBudget(t *testing.T) {
	handler, blobs := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Body flood page",
	})

	// Test config allows 1 MiB files; the junk part is far beyond any
	// reasonable field overhead on top of that.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	junk, err := mw.CreateFormFile("junk", "junk.bin")
	if err != nil {
		t.Fatalf("CreateFormFile junk: %v", err)
	}
	if _, err := junk.Write(bytes.Repeat([]byte("A"), 3<<20)); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	file, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatalf("CreateFormFile file: %v", err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	resp := perform(t, handler, http.MethodPost, "/api/u/"+slug, &buf, nil, map[string]string{
		"Content-Type": mw.FormDataContentType(),
	})
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", resp.Code, resp.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("blob was written despite oversized request: %d objects", len(blobs.objects))
	}
}

func TestSecurityHeadersAppliedToAllResponses(t *testing.T) {
	handler, _ := newTestHandler(t)

	resp := perform(t, handler, http.MethodGet, "/api/u/doesnotexist0000", nil, nil, nil)
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := resp.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	csp := resp.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("Content-Security-Policy = %q, missing %q", csp, directive)
		}
	}
	// The test config uses SecureCookies=true, i.e. an HTTPS deployment, so
	// strict transport must be advertised.
	if got := resp.Header().Get("Strict-Transport-Security"); !strings.Contains(got, "max-age=") {
		t.Errorf("Strict-Transport-Security = %q, want max-age directive", got)
	}
}

// Onion services are plain HTTP end to end; advertising HSTS there is
// meaningless noise, so the header follows the SecureCookies switch that
// already distinguishes the two deployment modes.
func TestHSTSOmittedForInsecureCookieDeployments(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.SecureCookies = false
	})
	resp := perform(t, handler, http.MethodGet, "/api/u/doesnotexist0000", nil, nil, nil)
	if got := resp.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want unset for insecure-cookie deployments", got)
	}
}

// sabotagingBlobStore lets a test trigger a store failure at the exact moment
// between the blob write succeeding and the upload row being recorded.
type sabotagingBlobStore struct {
	inner       *memoryBlobStore
	afterUpload func()
}

func (s *sabotagingBlobStore) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	if err := s.inner.Upload(ctx, key, body, contentType); err != nil {
		return err
	}
	if s.afterUpload != nil {
		s.afterUpload()
	}
	return nil
}

func (s *sabotagingBlobStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.inner.Download(ctx, key)
}

func (s *sabotagingBlobStore) Delete(ctx context.Context, key string) error {
	return s.inner.Delete(ctx, key)
}

// If recording the upload fails after the blob was written, the handler must
// delete the blob again: the client saw an error and will retry, and an
// unreferenced object would otherwise sit in storage forever, invisible to the
// admin UI and to cleanup.
func TestUploadDeletesBlobWhenRecordingFails(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sprag.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	blobs := &memoryBlobStore{objects: map[string][]byte{}}
	sabotage := &sabotagingBlobStore{inner: blobs, afterUpload: func() {
		raw, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Errorf("open raw connection: %v", err)
			return
		}
		defer raw.Close()
		if _, err := raw.ExecContext(ctx, "DROP TABLE custody_events"); err != nil {
			t.Errorf("drop custody_events: %v", err)
		}
	}}

	handler, err := httpapi.New(httpapi.Dependencies{
		Store:     db,
		BlobStore: sabotage,
		Config: httpapi.Config{
			BaseURL:       "https://sprag.example.test",
			SessionSecret: []byte("12345678901234567890123456789012"),
			AdminUsername: "admin",
			AdminPassword: "correct-password",
			MaxFileSize:   1024 * 1024,
			S3Prefix:      "pages/",
			SecureCookies: true,
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:  func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New handler failed: %v", err)
	}

	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Cleanup page",
	})

	resp := performMultipart(t, handler, "/api/u/"+slug, "file", "note.txt", []byte("hello"), nil)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", resp.Code, resp.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("blob survived failed upload recording: %d objects", len(blobs.objects))
	}
	uploads, err := db.ListUploads(ctx, 1)
	if err != nil {
		t.Fatalf("ListUploads failed: %v", err)
	}
	if len(uploads) != 0 {
		t.Fatalf("upload row survived failed recording: %#v", uploads)
	}
}
