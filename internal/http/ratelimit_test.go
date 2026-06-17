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

package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httpapi "github.com/tob/zener/internal/http"
)

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
