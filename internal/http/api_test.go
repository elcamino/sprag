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
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
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

	"github.com/elcamino/sprag/internal/blob"
	httpapi "github.com/elcamino/sprag/internal/http"
	"github.com/elcamino/sprag/internal/store"
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

func TestUploadsCanShareSubmissionEnvelope(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Envelope intake"})

	submissionID := "11111111-1111-4111-8111-111111111111"
	for _, name := range []string{"one.txt", "two.txt"} {
		resp := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
			"submission_id": submissionID,
		}, "file", name, []byte(name), nil)
		if resp.Code != http.StatusCreated {
			t.Fatalf("upload %q status = %d body=%s", name, resp.Code, resp.Body.String())
		}
		var created struct {
			SubmissionID string `json:"submission_id"`
		}
		decodeJSON(t, resp.Body.Bytes(), &created)
		if created.SubmissionID != submissionID {
			t.Fatalf("upload %q submission_id = %q, want %q", name, created.SubmissionID, submissionID)
		}
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	var listed []struct {
		Name                 string `json:"name"`
		SubmissionID         string `json:"submission_id"`
		SubmissionUploadedAt string `json:"submission_uploaded_at"`
	}
	decodeJSON(t, files.Body.Bytes(), &listed)
	if len(listed) != 2 {
		t.Fatalf("expected two files, got %d", len(listed))
	}
	for _, file := range listed {
		if file.SubmissionID != submissionID {
			t.Fatalf("listed file %q submission_id = %q, want %q", file.Name, file.SubmissionID, submissionID)
		}
		if file.SubmissionUploadedAt == "" {
			t.Fatalf("listed file %q missing submission_uploaded_at", file.Name)
		}
	}
}

func TestUploadReturnsStatusOnlyReceipt(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Receipt intake"})

	submissionID := "55555555-5555-4555-8555-555555555555"
	first := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"submission_id": submissionID,
	}, "file", "one.pdf", []byte("first body"), nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload status = %d body=%s", first.Code, first.Body.String())
	}
	var firstCreated struct {
		SubmissionID  string `json:"submission_id"`
		ReceiptURL    string `json:"receipt_url"`
		ReceiptStatus string `json:"receipt_status"`
	}
	decodeJSON(t, first.Body.Bytes(), &firstCreated)
	if firstCreated.SubmissionID != submissionID {
		t.Fatalf("submission_id = %q, want %q", firstCreated.SubmissionID, submissionID)
	}
	if firstCreated.ReceiptStatus != "received" {
		t.Fatalf("receipt_status = %q, want received", firstCreated.ReceiptStatus)
	}
	if !strings.HasPrefix(firstCreated.ReceiptURL, "https://sprag.example.test/r/") {
		t.Fatalf("receipt_url = %q, want public /r URL", firstCreated.ReceiptURL)
	}
	token := firstCreated.ReceiptURL[strings.LastIndex(firstCreated.ReceiptURL, "/")+1:]

	second := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"submission_id": submissionID,
	}, "file", "two.pdf", []byte("second body"), nil)
	if second.Code != http.StatusCreated {
		t.Fatalf("second upload status = %d body=%s", second.Code, second.Body.String())
	}
	var secondCreated struct {
		ReceiptURL string `json:"receipt_url"`
	}
	decodeJSON(t, second.Body.Bytes(), &secondCreated)
	if secondCreated.ReceiptURL != firstCreated.ReceiptURL {
		t.Fatalf("receipt_url changed within one submission: %q vs %q", secondCreated.ReceiptURL, firstCreated.ReceiptURL)
	}

	receipt := perform(t, handler, http.MethodGet, "/api/r/"+token, nil, nil, nil)
	if receipt.Code != http.StatusOK {
		t.Fatalf("receipt status = %d body=%s", receipt.Code, receipt.Body.String())
	}
	var publicReceipt struct {
		Status      string `json:"status"`
		SubmittedAt string `json:"submitted_at"`
		UpdatedAt   string `json:"updated_at"`
		FileCount   int64  `json:"file_count"`
		TotalSize   int64  `json:"total_size"`
	}
	decodeJSON(t, receipt.Body.Bytes(), &publicReceipt)
	if publicReceipt.Status != "received" || publicReceipt.FileCount != 2 || publicReceipt.TotalSize != 21 {
		t.Fatalf("receipt body = %#v, want received/2 files/21 bytes", publicReceipt)
	}
	body := receipt.Body.String()
	for _, forbidden := range []string{"one.pdf", "two.pdf", "Receipt intake", slug, submissionID, "uploader_ip"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public receipt leaked %q in %s", forbidden, body)
		}
	}

	missing := perform(t, handler, http.MethodGet, "/api/r/not-a-real-receipt-token", nil, nil, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing receipt status = %d body=%s, want 404", missing.Code, missing.Body.String())
	}
}

func TestAdminCanUpdateReceiptStatusOnly(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Receipt status"})

	submissionID := "66666666-6666-4666-8666-666666666666"
	upload := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"submission_id": submissionID,
	}, "file", "evidence.pdf", []byte("body"), nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	var created struct {
		ReceiptURL string `json:"receipt_url"`
	}
	decodeJSON(t, upload.Body.Bytes(), &created)
	token := created.ReceiptURL[strings.LastIndex(created.ReceiptURL, "/")+1:]

	update := performJSON(t, handler, http.MethodPatch, "/api/admin/pages/1/submissions/"+submissionID+"/receipt", map[string]string{
		"status": "reviewed",
	}, session, csrfHeader())
	if update.Code != http.StatusOK {
		t.Fatalf("status update = %d body=%s", update.Code, update.Body.String())
	}
	var updated struct {
		SubmissionID  string `json:"submission_id"`
		ReceiptURL    string `json:"receipt_url"`
		ReceiptStatus string `json:"receipt_status"`
	}
	decodeJSON(t, update.Body.Bytes(), &updated)
	if updated.SubmissionID != submissionID || updated.ReceiptURL != created.ReceiptURL || updated.ReceiptStatus != "reviewed" {
		t.Fatalf("unexpected status update body: %#v", updated)
	}

	receipt := perform(t, handler, http.MethodGet, "/api/r/"+token, nil, nil, nil)
	if receipt.Code != http.StatusOK {
		t.Fatalf("receipt status = %d body=%s", receipt.Code, receipt.Body.String())
	}
	if !strings.Contains(receipt.Body.String(), `"status":"reviewed"`) {
		t.Fatalf("receipt did not show reviewed status: %s", receipt.Body.String())
	}

	badStatus := performJSON(t, handler, http.MethodPatch, "/api/admin/pages/1/submissions/"+submissionID+"/receipt", map[string]string{
		"status": "message-sent",
	}, session, csrfHeader())
	if badStatus.Code != http.StatusBadRequest {
		t.Fatalf("invalid status update = %d body=%s, want 400", badStatus.Code, badStatus.Body.String())
	}
	if !strings.Contains(badStatus.Body.String(), "invalid_receipt_status") {
		t.Fatalf("expected invalid_receipt_status error, got %s", badStatus.Body.String())
	}
}

func TestAdminManifestIncludesStoredObjectHashesAndHandlingLog(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Manifest intake"})

	body := []byte("manifest body")
	upload := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"submission_id": "77777777-7777-4777-8777-777777777777",
	}, "file", "manifest.pdf", body, nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	wantHash := sha512Hex(body)

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	var listed []struct {
		ObjectSHA512        string `json:"object_sha512"`
		ObjectHashAlgorithm string `json:"object_hash_algorithm"`
	}
	decodeJSON(t, files.Body.Bytes(), &listed)
	if len(listed) != 1 {
		t.Fatalf("expected one file, got %d", len(listed))
	}
	if listed[0].ObjectHashAlgorithm != "SHA-512" || listed[0].ObjectSHA512 != wantHash {
		t.Fatalf("listed hash = %q/%q, want SHA-512/%q", listed[0].ObjectHashAlgorithm, listed[0].ObjectSHA512, wantHash)
	}

	download := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files/1", nil, session, nil)
	if download.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", download.Code, download.Body.String())
	}

	manifest := perform(t, handler, http.MethodGet, "/api/admin/pages/1/manifest", nil, session, nil)
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest status = %d body=%s", manifest.Code, manifest.Body.String())
	}
	if got := manifest.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("manifest content type = %q, want JSON", got)
	}
	var data struct {
		Version int `json:"version"`
		Page    struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"page"`
		Files []struct {
			ID                  int64  `json:"id"`
			SubmissionID        string `json:"submission_id"`
			Name                string `json:"name"`
			ObjectKey           string `json:"object_key"`
			ObjectSHA512        string `json:"object_sha512"`
			ObjectHashAlgorithm string `json:"object_hash_algorithm"`
			DownloadedAt        string `json:"downloaded_at"`
		} `json:"files"`
		HandlingLog []struct {
			EventType    string `json:"event_type"`
			Actor        string `json:"actor"`
			UploadID     int64  `json:"upload_id,omitempty"`
			SubmissionID string `json:"submission_id,omitempty"`
			CreatedAt    string `json:"created_at"`
		} `json:"handling_log"`
	}
	decodeJSON(t, manifest.Body.Bytes(), &data)
	if data.Version != 1 || data.Page.ID != 1 || data.Page.Title != "Manifest intake" {
		t.Fatalf("unexpected manifest header: %#v", data)
	}
	if len(data.Files) != 1 {
		t.Fatalf("expected one manifest file, got %d", len(data.Files))
	}
	file := data.Files[0]
	if file.Name != "manifest.pdf" || file.SubmissionID == "" || file.ObjectKey == "" || file.DownloadedAt == "" {
		t.Fatalf("manifest file missing expected metadata: %#v", file)
	}
	if file.ObjectHashAlgorithm != "SHA-512" || file.ObjectSHA512 != wantHash {
		t.Fatalf("manifest hash = %q/%q, want SHA-512/%q", file.ObjectHashAlgorithm, file.ObjectSHA512, wantHash)
	}
	events := map[string]bool{}
	for _, event := range data.HandlingLog {
		if event.CreatedAt == "" {
			t.Fatalf("event missing created_at: %#v", event)
		}
		events[event.EventType] = true
	}
	for _, want := range []string{"upload.accepted", "file.downloaded"} {
		if !events[want] {
			t.Fatalf("manifest missing %s event in %#v", want, data.HandlingLog)
		}
	}

	var created struct {
		ReceiptURL string `json:"receipt_url"`
	}
	decodeJSON(t, upload.Body.Bytes(), &created)
	token := created.ReceiptURL[strings.LastIndex(created.ReceiptURL, "/")+1:]
	receipt := perform(t, handler, http.MethodGet, "/api/r/"+token, nil, nil, nil)
	if receipt.Code != http.StatusOK {
		t.Fatalf("receipt status = %d body=%s", receipt.Code, receipt.Body.String())
	}
	if strings.Contains(receipt.Body.String(), wantHash) || strings.Contains(receipt.Body.String(), "sha512") || strings.Contains(receipt.Body.String(), "SHA-512") {
		t.Fatalf("public receipt leaked hash material: %s", receipt.Body.String())
	}
}

func TestE2EManifestHashesStoredCiphertextWithoutPlaintextName(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.E2EIntake.Enabled = true
		c.E2EIntake.Algorithm = "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM"
	})
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title":                      "Encrypted manifest",
		"e2e_public_key":             `{"sprag":"e2e-public-key","version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","publicKey":"pub","fingerprint":"sha256:test"}`,
		"e2e_public_key_fingerprint": "sha256:test",
		"e2e_algorithm":              "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM",
	})
	envelope := `{"version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","public_key_fingerprint":"sha256:test","kem_ciphertext":"Y3Q","ecdh_ephemeral_public_key":"e30","salt":"c2FsdA","file_nonce":"ZmlsZQ","metadata_nonce":"bWV0YQ","encrypted_metadata":"c2VhbGVk"}`
	ciphertext := []byte("opaque ciphertext")
	upload := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"e2e_envelope": envelope,
	}, "file", "privileged-report.pdf", ciphertext, nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("encrypted upload status = %d body=%s", upload.Code, upload.Body.String())
	}

	manifest := perform(t, handler, http.MethodGet, "/api/admin/pages/1/manifest", nil, session, nil)
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest status = %d body=%s", manifest.Code, manifest.Body.String())
	}
	if strings.Contains(manifest.Body.String(), "privileged-report.pdf") {
		t.Fatalf("manifest leaked plaintext E2E filename: %s", manifest.Body.String())
	}
	var data struct {
		Files []struct {
			Name                string `json:"name"`
			EncryptionMode      string `json:"encryption_mode"`
			ObjectSHA512        string `json:"object_sha512"`
			ObjectHashAlgorithm string `json:"object_hash_algorithm"`
			ObjectHashScope     string `json:"object_hash_scope"`
		} `json:"files"`
	}
	decodeJSON(t, manifest.Body.Bytes(), &data)
	if len(data.Files) != 1 {
		t.Fatalf("expected one manifest file, got %d", len(data.Files))
	}
	file := data.Files[0]
	if file.EncryptionMode != "e2e-v1" || file.ObjectHashScope != "stored-ciphertext" {
		t.Fatalf("unexpected E2E manifest file metadata: %#v", file)
	}
	if file.ObjectHashAlgorithm != "SHA-512" || file.ObjectSHA512 != sha512Hex(ciphertext) {
		t.Fatalf("manifest hash = %q/%q, want SHA-512/%q", file.ObjectHashAlgorithm, file.ObjectSHA512, sha512Hex(ciphertext))
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

func TestUploadStoresPlainIPByDefault(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Plain IP intake"})

	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "plain.txt", []byte("hello"), nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	var listed []struct {
		UploaderIP string `json:"uploader_ip"`
	}
	decodeJSON(t, files.Body.Bytes(), &listed)
	if len(listed) != 1 {
		t.Fatalf("expected one listed file, got %d", len(listed))
	}
	if listed[0].UploaderIP != "192.0.2.1" {
		t.Fatalf("uploader_ip = %q, want plaintext test peer IP", listed[0].UploaderIP)
	}
}

func TestUploadStoresHMACIPIdentifierWhenConfigured(t *testing.T) {
	secret := []byte("12345678901234567890123456789012")
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.IPStorageMode = "hmac-sha256"
		c.IPHashSecret = secret
	})
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "Hashed IP intake"})

	upload := performMultipart(t, handler, "/api/u/"+slug, "file", "hashed.txt", []byte("hello"), nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	var listed []struct {
		UploaderIP string `json:"uploader_ip"`
	}
	decodeJSON(t, files.Body.Bytes(), &listed)
	if len(listed) != 1 {
		t.Fatalf("expected one listed file, got %d", len(listed))
	}
	want := expectedIPDigest(secret, "192.0.2.1")
	if listed[0].UploaderIP != want {
		t.Fatalf("uploader_ip = %q, want %q", listed[0].UploaderIP, want)
	}
	if strings.Contains(listed[0].UploaderIP, "192.0.2.1") {
		t.Fatalf("hashed uploader_ip leaked plaintext IP: %q", listed[0].UploaderIP)
	}
}

func TestNewRewritesExistingUploaderIPsWhenHMACStorageConfigured(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()
	page, err := db.CreatePage(ctx, store.PageCreate{Slug: "hash-old-ips-1", Title: "Existing"})
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}
	submissionID := "33333333-3333-4333-8333-333333333333"
	if _, err := db.CreateUpload(ctx, store.UploadCreate{
		PageID:       page.ID,
		S3Key:        "pages/existing/file.txt",
		OriginalName: "file.txt",
		SizeBytes:    5,
		UploaderIP:   "198.51.100.77",
		SubmissionID: submissionID,
	}); err != nil {
		t.Fatalf("CreateUpload failed: %v", err)
	}

	secret := []byte("12345678901234567890123456789012")
	_, err = httpapi.New(httpapi.Dependencies{
		Store:     db,
		BlobStore: &memoryBlobStore{objects: map[string][]byte{}},
		Config: httpapi.Config{
			BaseURL:           "https://sprag.example.test",
			SessionSecret:     []byte("12345678901234567890123456789012"),
			AdminUsername:     "admin",
			AdminPassword:     "correct-password",
			MaxFileSize:       1024 * 1024,
			S3Prefix:          "pages/",
			IPStorageMode:     "hmac-sha256",
			IPHashSecret:      secret,
			AllowedExtensions: nil,
			SecureCookies:     true,
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	want := expectedIPDigest(secret, "198.51.100.77")
	files, err := db.ListUploads(ctx, page.ID)
	if err != nil {
		t.Fatalf("ListUploads failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one upload, got %d", len(files))
	}
	if files[0].UploaderIP != want {
		t.Fatalf("upload uploader_ip = %q, want %q", files[0].UploaderIP, want)
	}
	envelope, err := db.GetSubmissionEnvelope(ctx, page.ID, submissionID)
	if err != nil {
		t.Fatalf("GetSubmissionEnvelope failed: %v", err)
	}
	if envelope.UploaderIP != want {
		t.Fatalf("envelope uploader_ip = %q, want %q", envelope.UploaderIP, want)
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

func TestLoginAcceptsPrecomputedPasswordHash(t *testing.T) {
	hash, err := httpapi.HashAdminPassword("hashed-secret")
	if err != nil {
		t.Fatalf("HashAdminPassword failed: %v", err)
	}
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AdminPassword = ""
		c.AdminPasswordHash = hash
	})

	ok := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "hashed-secret",
	}, nil, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("login with correct password against hash status = %d, want 200", ok.Code)
	}

	bad := performJSON(t, handler, http.MethodPost, "/api/admin/login", map[string]string{
		"username": "admin",
		"password": "wrong-secret",
	}, nil, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("login with wrong password against hash status = %d, want 401", bad.Code)
	}
}

func TestNewRejectsInvalidPasswordHash(t *testing.T) {
	_, _, err := newHandlerErr(t, func(c *httpapi.Config) {
		c.AdminPassword = ""
		c.AdminPasswordHash = "not-a-bcrypt-hash"
	})
	if err == nil {
		t.Fatal("expected invalid password hash to fail")
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

func TestE2EPagePublishesPublicKeyAndStoresEncryptedUploadMetadata(t *testing.T) {
	handler, blobs := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.E2EIntake.Enabled = true
		c.E2EIntake.Algorithm = "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM"
	})
	session := loginAdmin(t, handler)

	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title":                      "Encrypted intake",
		"e2e_public_key":             `{"sprag":"e2e-public-key","version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","publicKey":"pub","fingerprint":"sha256:test"}`,
		"e2e_public_key_fingerprint": "sha256:test",
		"e2e_algorithm":              "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM",
	}, session, csrfHeader())
	if create.Code != http.StatusCreated {
		t.Fatalf("create encrypted page status = %d body=%s", create.Code, create.Body.String())
	}
	var page struct {
		ID                      int64  `json:"id"`
		URL                     string `json:"url"`
		E2EPublicKey            string `json:"e2e_public_key"`
		E2EPublicKeyFingerprint string `json:"e2e_public_key_fingerprint"`
		E2EAlgorithm            string `json:"e2e_algorithm"`
	}
	decodeJSON(t, create.Body.Bytes(), &page)
	if page.E2EPublicKey == "" || page.E2EPublicKeyFingerprint != "sha256:test" {
		t.Fatalf("encrypted page did not return its public identity: %#v", page)
	}
	slug := page.URL[strings.LastIndex(page.URL, "/")+1:]

	meta := perform(t, handler, http.MethodGet, "/api/u/"+slug, nil, nil, nil)
	if meta.Code != http.StatusOK {
		t.Fatalf("metadata status = %d body=%s", meta.Code, meta.Body.String())
	}
	var public struct {
		E2E *struct {
			Enabled              bool   `json:"enabled"`
			Algorithm            string `json:"algorithm"`
			PublicKey            string `json:"public_key"`
			PublicKeyFingerprint string `json:"public_key_fingerprint"`
		} `json:"e2e"`
	}
	decodeJSON(t, meta.Body.Bytes(), &public)
	if public.E2E == nil || !public.E2E.Enabled || public.E2E.PublicKeyFingerprint != "sha256:test" {
		t.Fatalf("public metadata missing E2E identity: %s", meta.Body.String())
	}

	envelope := `{"version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","public_key_fingerprint":"sha256:test","kem_ciphertext":"Y3Q","ecdh_ephemeral_public_key":"e30","salt":"c2FsdA","file_nonce":"ZmlsZQ","metadata_nonce":"bWV0YQ","encrypted_metadata":"c2VhbGVk"}`
	upload := performMultipartFields(t, handler, "/api/u/"+slug, map[string]string{
		"e2e_envelope": envelope,
	}, "file", "privileged-report.pdf", []byte("ciphertext"), nil)
	if upload.Code != http.StatusCreated {
		t.Fatalf("encrypted upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	for key := range blobs.objects {
		if strings.Contains(key, "privileged-report.pdf") {
			t.Fatalf("object key leaked plaintext filename: %q", key)
		}
		if !strings.HasSuffix(key, ".sprag") {
			t.Fatalf("encrypted object key suffix = %q, want .sprag", key)
		}
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusOK {
		t.Fatalf("files status = %d body=%s", files.Code, files.Body.String())
	}
	if strings.Contains(files.Body.String(), "privileged-report.pdf") {
		t.Fatalf("file list leaked plaintext filename: %s", files.Body.String())
	}
	if !strings.Contains(files.Body.String(), `"encryption_mode":"e2e-v1"`) || !strings.Contains(files.Body.String(), `"encryption_envelope"`) {
		t.Fatalf("file list missing encrypted upload envelope: %s", files.Body.String())
	}
}

func TestE2ERequiredRejectsPlainPagesAndPlainUploads(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.E2EIntake.Enabled = true
		c.E2EIntake.Required = true
		c.E2EIntake.Algorithm = "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM"
	})
	session := loginAdmin(t, handler)

	plainPage := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title": "Plain",
	}, session, csrfHeader())
	if plainPage.Code != http.StatusBadRequest {
		t.Fatalf("plain page status = %d body=%s, want 400", plainPage.Code, plainPage.Body.String())
	}
	if !strings.Contains(plainPage.Body.String(), "e2e_required") {
		t.Fatalf("expected e2e_required error, got %s", plainPage.Body.String())
	}

	slug := createPageSlug(t, handler, session, map[string]any{
		"title":                      "Encrypted",
		"e2e_public_key":             `{"sprag":"e2e-public-key","version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","publicKey":"pub","fingerprint":"sha256:test"}`,
		"e2e_public_key_fingerprint": "sha256:test",
		"e2e_algorithm":              "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM",
	})
	plainUpload := performMultipart(t, handler, "/api/u/"+slug, "file", "secret.pdf", []byte("plaintext"), nil)
	if plainUpload.Code != http.StatusBadRequest {
		t.Fatalf("plain upload status = %d body=%s, want 400", plainUpload.Code, plainUpload.Body.String())
	}
	if !strings.Contains(plainUpload.Body.String(), "e2e_required") {
		t.Fatalf("expected e2e_required error, got %s", plainUpload.Body.String())
	}
}

func TestE2EDisabledRejectsEncryptedPageIdentity(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)

	create := performJSON(t, handler, http.MethodPost, "/api/admin/pages", map[string]any{
		"title":                      "Encrypted",
		"e2e_public_key":             `{"sprag":"e2e-public-key","version":1,"algorithm":"ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM","publicKey":"pub","fingerprint":"sha256:test"}`,
		"e2e_public_key_fingerprint": "sha256:test",
		"e2e_algorithm":              "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM",
	}, session, csrfHeader())
	if create.Code != http.StatusBadRequest {
		t.Fatalf("encrypted page while disabled status = %d body=%s, want 400", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "e2e_disabled") {
		t.Fatalf("expected e2e_disabled error, got %s", create.Body.String())
	}
}

func TestListFilesForDeletedPageReturnsNotFound(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	createPageSlug(t, handler, session, map[string]any{"title": "encryption test"})

	deletePage := perform(t, handler, http.MethodDelete, "/api/admin/pages/1", nil, session, csrfHeader())
	if deletePage.Code != http.StatusNoContent {
		t.Fatalf("delete page status = %d body=%s", deletePage.Code, deletePage.Body.String())
	}

	files := perform(t, handler, http.MethodGet, "/api/admin/pages/1/files", nil, session, nil)
	if files.Code != http.StatusNotFound {
		t.Fatalf("files for deleted page status = %d body=%s, want 404", files.Code, files.Body.String())
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

	// Valid session cookie but no X-Sprag-CSRF header.
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
	handler, _, err := newHandlerWithErr(t, blobs, tweak)
	if err != nil {
		t.Fatalf("New handler failed: %v", err)
	}
	return handler
}

func newHandlerErr(t *testing.T, tweak func(*httpapi.Config)) (http.Handler, *memoryBlobStore, error) {
	blobs := &memoryBlobStore{objects: map[string][]byte{}}
	handler, _, err := newHandlerWithErr(t, blobs, tweak)
	return handler, blobs, err
}

func newHandlerWithErr(t *testing.T, blobs blob.Store, tweak func(*httpapi.Config)) (http.Handler, blob.Store, error) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "sprag.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := httpapi.Config{
		BaseURL:           "https://sprag.example.test",
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
	return handler, blobs, err
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
	return performMultipartFields(t, h, path, nil, field, filename, content, cookies)
}

func performMultipartFields(t *testing.T, h http.Handler, path string, fields map[string]string, field, filename string, content []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			t.Fatalf("WriteField %s: %v", name, err)
		}
	}
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

func expectedIPDigest(secret []byte, ip string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ip))
	return "ip-hmac-sha256:v1:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func sha512Hex(data []byte) string {
	sum := sha512.Sum512(data)
	return hex.EncodeToString(sum[:])
}

func csrfHeader() map[string]string {
	return map[string]string{"X-Sprag-CSRF": "1"}
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
