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
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	httpapi "github.com/elcamino/sprag/internal/http"
)

// attemptUpload posts a minimal multipart upload to a page from a chosen peer
// address so rate-limit bucketing by client IP can be exercised.
func attemptUpload(t *testing.T, handler http.Handler, slug, remoteAddr string) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/u/"+slug, &buf)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Code
}

// The public upload endpoint is the core attack surface of a file drop: without
// a limiter an attacker who knows a slug can fill storage as fast as the network
// allows. Buckets are scoped per page and client IP so one abusive client does
// not lock out uploaders arriving from other addresses.
func TestUploadRateLimitIsPageAndClientScoped(t *testing.T) {
	handler, _ := newTestHandler(t)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Upload rate limited page",
	})

	for i := 0; i < 30; i++ {
		if code := attemptUpload(t, handler, slug, "198.51.100.7:1234"); code != http.StatusCreated {
			t.Fatalf("upload %d = %d, want 201", i+1, code)
		}
	}
	if code := attemptUpload(t, handler, slug, "198.51.100.7:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("31st upload from same client = %d, want 429", code)
	}
	if code := attemptUpload(t, handler, slug, "203.0.113.9:1234"); code != http.StatusCreated {
		t.Fatalf("upload from different client = %d, want 201 (buckets must be per client)", code)
	}
}

// Under anonymous ingress no client identifier exists, so the upload limit must
// degrade to a single page-scoped bucket that a rotating peer address cannot
// escape — on Tor the apparent peer is meaningless.
func TestAnonymousIngressUploadRateLimitIsPageScoped(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AnonymousIngress = true
	})
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Anonymous upload page",
	})

	for i := 0; i < 30; i++ {
		remote := fmt.Sprintf("198.51.100.%d:1234", i+1)
		if code := attemptUpload(t, handler, slug, remote); code != http.StatusCreated {
			t.Fatalf("upload %d = %d, want 201", i+1, code)
		}
	}
	if code := attemptUpload(t, handler, slug, "203.0.113.9:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("31st upload from a different apparent peer = %d, want 429", code)
	}
}

// With one trusted proxy hop, an attacker who rotates the spoofable left side of
// X-Forwarded-For must NOT be able to mint fresh login rate-limit buckets: the
// real client IP is the entry the proxy appended on the right. After five
// attempts from the same proxy-observed IP the sixth must be rejected, regardless
// of the spoofed prefix.
func TestLoginRateLimitSurvivesForwardedForSpoofing(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.TrustedProxyHops = 1
	})

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong-password"})
	attempt := func(spoof string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// Each request carries a different spoofed prefix but the same real client
		// IP appended by the trusted proxy.
		req.Header.Set("X-Forwarded-For", spoof+", 198.51.100.9")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	spoofs := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"}
	for i, spoof := range spoofs {
		if code := attempt(spoof); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d (%s) = %d, want 401", i+1, spoof, code)
		}
	}
	if code := attempt("6.6.6.6"); code != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt = %d, want 429 (spoofed prefix must not bypass the limit)", code)
	}
}

func TestAnonymousIngressLoginRateLimitIsGlobal(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AnonymousIngress = true
	})

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong-password"})
	attempt := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
		req.RemoteAddr = remoteAddr
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	for i := 0; i < 5; i++ {
		remote := fmt.Sprintf("198.51.100.%d:1234", i+1)
		if code := attempt(remote); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, code)
		}
	}
	if code := attempt("203.0.113.9:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt from a different apparent peer = %d, want 429", code)
	}
}

func TestAnonymousIngressPINRateLimitIsPageScoped(t *testing.T) {
	handler, _ := newTestHandlerWithConfig(t, func(c *httpapi.Config) {
		c.AnonymousIngress = true
	})
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{
		"title": "Anonymous PIN page",
		"pin":   "2468",
	})

	attempt := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/u/"+slug+"/pin", bytes.NewReader([]byte(`{"pin":"wrong"}`)))
		req.RemoteAddr = remoteAddr
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	for i := 0; i < 10; i++ {
		remote := fmt.Sprintf("198.51.100.%d:1234", i+1)
		if code := attempt(remote); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, code)
		}
	}
	if code := attempt("203.0.113.9:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("eleventh attempt from a different apparent peer = %d, want 429", code)
	}
}
