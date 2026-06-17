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

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAFallbackServesIndexWithoutRedirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	spaHandler(http.Dir(dir)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with location %q", rr.Code, rr.Header().Get("Location"))
	}
	if rr.Body.String() != "<div id=\"root\"></div>" {
		t.Fatalf("unexpected body %q", rr.Body.String())
	}
}

func TestFrontendIndexDisallowsIndexing(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "index.html"))
	if err != nil {
		t.Fatalf("read frontend index: %v", err)
	}
	html := strings.ToLower(string(body))
	if !strings.Contains(html, `<meta name="robots" content="noindex,nofollow"`) {
		t.Fatalf("frontend index.html must include a robots noindex,nofollow meta tag")
	}
}

func TestFrontendRobotsDisallowsEverything(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "public", "robots.txt"))
	if err != nil {
		t.Fatalf("read frontend robots.txt: %v", err)
	}
	got := strings.TrimSpace(string(body))
	want := "User-agent: *\nDisallow: /"
	if got != want {
		t.Fatalf("unexpected robots.txt content\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
