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

package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tob/zener/internal/blob"
	httpapi "github.com/tob/zener/internal/http"
	"github.com/tob/zener/internal/store"
)

func TestAdminCreatesPageUploaderUploadsAndAdminDownloads(t *testing.T) {
	handler, blobs := newTestHandler(t)

	login := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "correct-password",
	}, nil, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	session := login.Result().Cookies()

	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title":         "Client intake",
		"description":   "Upload PDFs here",
		"allowed_ext":   "pdf",
		"max_file_size": 1024,
	}, session, csrfHeader())
	if create.Code != http.StatusCreated {
		t.Fatalf("create page status = %d body=%s", create.Code, create.Body.String())
	}
	var page struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	if page.ID == 0 || !strings.Contains(page.URL, "/u/") {
		t.Fatalf("unexpected page response: %#v", page)
	}
	slug := page.URL[strings.LastIndex(page.URL, "/")+1:]

	meta := perform(t, handler, http.MethodGet, "/api/u/"+slug, nil, nil, nil)
	if meta.Code != http.StatusOK {
		t.Fatalf("metadata status = %d body=%s", meta.Code, meta.Body.String())
	}
	if strings.Contains(meta.Body.String(), "upload_count") {
		t.Fatalf("public metadata leaked upload counts: %s", meta.Body.String())
	}

	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "report.pdf", []byte("hello world"), nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	if len(blobs.objects) != 1 {
		t.Fatalf("expected one object write, got %d", len(blobs.objects))
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	if !strings.Contains(files.Body.String(), "report.pdf") {
		t.Fatalf("file list missing uploaded file: %s", files.Body.String())
	}

	download := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files/1", nil, session, nil)
	if download.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", download.Code, download.Body.String())
	}
	if got := download.Header().Get("Content-Disposition"); !strings.Contains(got, `attachment`) || !strings.Contains(got, `report.pdf`) {
		t.Fatalf("unexpected content disposition %q", got)
	}
	if download.Body.String() != "hello world" {
		t.Fatalf("unexpected download body %q", download.Body.String())
	}

	zipResp := perform(t, handler, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	if zipResp.Code != http.StatusOK {
		t.Fatalf("zip status = %d body=%s", zipResp.Code, zipResp.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(zipResp.Body.Bytes()), int64(zipResp.Body.Len()))
	if err != nil {
		t.Fatalf("zip did not open: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("expected one zip entry, got %d", len(zr.File))
	}
	if zr.File[0].Method != zip.Store {
		t.Fatalf("expected zip store method, got %d", zr.File[0].Method)
	}
}

func TestUploadRejectsDisallowedExtensionWithoutWritingBlob(t *testing.T) {
	handler, blobs := newTestHandler(t)
	session := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "correct-password",
	}, nil, nil).Result().Cookies()
	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title":       "PDFs",
		"allowed_ext": "pdf",
	}, session, csrfHeader())
	var page struct {
		URL string `json:"url"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	slug := page.URL[strings.LastIndex(page.URL, "/")+1:]

	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "invoice.exe", []byte("not a pdf"), nil)
	if upload.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("expected no blob writes, got %d", len(blobs.objects))
	}
	if !strings.Contains(upload.Body.String(), "extension_not_allowed") {
		t.Fatalf("expected extension error, got %s", upload.Body.String())
	}
}

func TestAdminListPagesReturnsEmptyArray(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "correct-password",
	}, nil, nil).Result().Cookies()

	pages := perform(t, handler, http.MethodGet, "/api/admin/pages", nil, session, nil)
	if pages.Code != http.StatusOK {
		t.Fatalf("pages status = %d body=%s", pages.Code, pages.Body.String())
	}
	if strings.TrimSpace(pages.Body.String()) != "[]" {
		t.Fatalf("expected empty JSON array, got %s", pages.Body.String())
	}
}

func TestPinnedPageRequiresPinCookie(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "correct-password",
	}, nil, nil).Result().Cookies()
	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title": "PIN page",
		"pin":   "2468",
	}, session, csrfHeader())
	var page struct {
		URL string `json:"url"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	slug := page.URL[strings.LastIndex(page.URL, "/")+1:]

	withoutPin := performMultipart(t, handler, "/api/u/"+slug, "file", "ok.txt", []byte("secret"), nil)
	if withoutPin.Code != http.StatusForbidden {
		t.Fatalf("upload without pin status = %d body=%s", withoutPin.Code, withoutPin.Body.String())
	}

	pin := performJSON(t, handler, http.MethodPost, "/api/u/"+slug+"/pin", map[string]string{"pin": "2468"}, nil, nil)
	if pin.Code != http.StatusOK {
		t.Fatalf("pin status = %d body=%s", pin.Code, pin.Body.String())
	}
	withPin := performMultipart(t, handler, "/api/u/"+slug, "file", "ok.txt", []byte("secret"), pin.Result().Cookies())
	if withPin.Code != http.StatusCreated {
		t.Fatalf("upload with pin status = %d body=%s", withPin.Code, withPin.Body.String())
	}
}

func TestUploadRejectsOversizedFileWithoutWritingBlob(t *testing.T) {
	handler, blobs := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title":         "Tiny",
		"max_file_size": 8,
	})

	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "big.bin", []byte("way more than eight bytes"), nil)
	if upload.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	if !strings.Contains(upload.Body.String(), "file_too_large") {
		t.Fatalf("expected file_too_large error, got %s", upload.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("expected no blob writes for rejected upload, got %d", len(blobs.objects))
	}
}

func TestLoginDistinguishesPasswordsBeyond72Bytes(t *testing.T) {
	long := strings.Repeat("a", 72)
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AdminPassword = long + "X"
	})
	// A different password sharing the first 72 bytes must not authenticate
	// (raw bcrypt would truncate both to the same 72 bytes and accept it).
	resp := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": long + "Y",
	}, nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("login with truncated-equal password status = %d, want 401", resp.Code)
	}
}

func TestUploadEnforcesGlobalExtensionCeiling(t *testing.T) {
	handler, blobs := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AllowedExtensions = []string{"pdf", "png"}
	})
	session := loginAdmin(t, handler)
	// The page tries to allow exe, which is outside the global ceiling.
	slug := createPageSlug(t, handler, session, map[string]any{
		"title":       "Ceiling",
		"allowed_ext": "exe,pdf",
	})

	exe := performMultipart(t, handler, "/api/u/"+slug, "file", "tool.exe", []byte("MZ"), nil)
	if exe.Code != http.StatusBadRequest {
		t.Fatalf("exe upload status = %d body=%s, want 400 (outside global ceiling)", exe.Code, exe.Body.String())
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("expected no blob writes for rejected upload, got %d", len(blobs.objects))
	}

	pdf := performMultipart(t, handler, "/api/u/"+slug, "file", "doc.pdf", []byte("%PDF"), nil)
	if pdf.Code != http.StatusCreated {
		t.Fatalf("pdf upload status = %d body=%s, want 201 (within ceiling)", pdf.Code, pdf.Body.String())
	}
}

func TestExpiredPageRejectsAccess(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	// Clock is pinned to 2026-06-16T12:00:00Z; this expiry is in the past.
	slug := createPageSlug(t, handler, session, map[string]any{
		"title":      "Expired",
		"expires_at": "2026-06-16T11:00:00Z",
	})

	meta := perform(t, handler, http.MethodGet, "/api/u/"+slug, nil, nil, nil)
	if meta.Code != http.StatusNotFound {
		t.Fatalf("expired page metadata status = %d body=%s", meta.Code, meta.Body.String())
	}
	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "late.txt", []byte("too late"), nil)
	if upload.Code != http.StatusNotFound {
		t.Fatalf("expired page upload status = %d body=%s", upload.Code, upload.Body.String())
	}
}

func TestInactivePageRejectsAccess(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{"title": "Toggle"}, session, csrfHeader())
	var page struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	slug := page.URL[strings.LastIndex(page.URL, "/")+1:]

	patch := performJSON(t, handler, http.MethodPatch, "/api/admin/pages/1", map[string]any{"is_active": false}, session, csrfHeader())
	if patch.Code != http.StatusOK {
		t.Fatalf("deactivate status = %d body=%s", patch.Code, patch.Body.String())
	}
	meta := perform(t, handler, http.MethodGet, "/api/u/"+slug, nil, nil, nil)
	if meta.Code != http.StatusNotFound {
		t.Fatalf("inactive page metadata status = %d body=%s", meta.Code, meta.Body.String())
	}
}

func TestAdminMutationRequiresCSRFHeader(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)

	// Valid session cookie but no X-Zener-CSRF header.
	resp := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{"title": "No CSRF"}, session, nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF header status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "csrf_required") {
		t.Fatalf("expected csrf_required error, got %s", resp.Body.String())
	}
}

func TestZipDeduplicatesDuplicateFilenames(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Dupes"})

	for _, content := range [][]byte{[]byte("first"), []byte("second")} {
		up := performMultipart(t, handler, "/api/u/"+slug, "file", "report.txt", content, nil)
		if up.Code != http.StatusCreated {
			t.Fatalf("upload status = %d body=%s", up.Code, up.Body.String())
		}
	}

	zipResp := perform(t, handler, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	if zipResp.Code != http.StatusOK {
		t.Fatalf("zip status = %d body=%s", zipResp.Code, zipResp.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(zipResp.Body.Bytes()), int64(zipResp.Body.Len()))
	if err != nil {
		t.Fatalf("zip did not open: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("expected two zip entries, got %d", len(zr.File))
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		if names[f.Name] {
			t.Fatalf("duplicate entry name in zip: %q", f.Name)
		}
		names[f.Name] = true
	}
}

func TestZipDeduplicationAvoidsCollidingWithRealName(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Tricky"})

	// Two "report.pdf" plus a genuine "report-2.pdf": the dedup-generated name
	// for the second report.pdf must not collide with the real report-2.pdf.
	uploads := []struct{ name, body string }{
		{"report.pdf", "one"},
		{"report.pdf", "two"},
		{"report-2.pdf", "three"},
	}
	for _, u := range uploads {
		up := performMultipart(t, handler, "/api/u/"+slug, "file", u.name, []byte(u.body), nil)
		if up.Code != http.StatusCreated {
			t.Fatalf("upload %q status = %d body=%s", u.name, up.Code, up.Body.String())
		}
	}

	zipResp := perform(t, handler, http.MethodGet, "/api/admin/pages/1/zip", nil, session, nil)
	if zipResp.Code != http.StatusOK {
		t.Fatalf("zip status = %d body=%s", zipResp.Code, zipResp.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(zipResp.Body.Bytes()), int64(zipResp.Body.Len()))
	if err != nil {
		t.Fatalf("zip did not open: %v", err)
	}
	if len(zr.File) != 3 {
		t.Fatalf("expected three zip entries, got %d", len(zr.File))
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		if names[f.Name] {
			t.Fatalf("duplicate entry name in zip: %q (names so far: %v)", f.Name, names)
		}
		names[f.Name] = true
	}
}

func loginAdmin(t *testing.T, handler http.Handler) []*http.Cookie {
	t.Helper()
	login := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "correct-password",
	}, nil, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	return login.Result().Cookies()
}

func createPageSlug(t *testing.T, handler http.Handler, session []*http.Cookie, body map[string]any) string {
	t.Helper()
	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", body, session, csrfHeader())
	if create.Code != http.StatusCreated {
		t.Fatalf("create page status = %d body=%s", create.Code, create.Body.String())
	}
	var page struct {
		URL string `json:"url"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	return page.URL[strings.LastIndex(page.URL, "/")+1:]
}

func newTestHandler(t *testing.T) (http.Handler, *memoryBlobStore) {
	return newTestHandlerWithConfig(t, nil)
}

func newTestHandlerWithConfig(t *testing.T, tweak func(*httpapi.Config)) (http.Handler, *memoryBlobStore) {
	blobs := &memoryBlobStore{objects: map[string][]byte{}}
	return newHandlerWith(t, blobs, tweak), blobs
}

func newHandlerWith(t *testing.T, blobs blob.Store, tweak func(*httpapi.Config)) http.Handler {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "zener.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := httpapi.Config{
		BaseURL:           "https://zener.example.test",
		SessionSecret:     []byte("12345678901234567890123456789012"),
		AdminUsername:     "admin",
		AdminPassword:     "correct-password",
		MaxFileSize:       1024 * 1024,
		AllowedExtensions: nil,
		S3Prefix:          "pages/",
		SecureCookies:     true,
	}
	if tweak != nil {
		tweak(&cfg)
	}
	handler, err := httpapi.New(httpapi.Dependencies{
		Store:     db,
		BlobStore: blobs,
		Config:    cfg,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:     func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New handler failed: %v", err)
	}
	return handler
}

type memoryBlobStore struct {
	objects map[string][]byte
}

func (m *memoryBlobStore) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *memoryBlobStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, httpapi.ErrBlobNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memoryBlobStore) Delete(ctx context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

func performJSON(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return perform(t, h, method, path, &buf, cookies, mergeHeaders(headers, map[string]string{"Content-Type": "application/json"}))
}

func performMultipart(t *testing.T, h http.Handler, path, field, filename string, content []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return perform(t, h, http.MethodPost, path, &buf, cookies, map[string]string{"Content-Type": mw.FormDataContentType()})
}

func perform(t *testing.T, h http.Handler, method, path string, body io.Reader, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, data []byte, dest any) {
	t.Helper()
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("decode JSON %s: %v", string(data), err)
	}
}

func csrfHeader() map[string]string {
	return map[string]string{"X-Zener-CSRF": "1"}
}

func mergeHeaders(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
