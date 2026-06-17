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
	"net/http"
	"testing"
)

// A download whose body errors mid-copy must abort the connection rather than
// return a 200 with a truncated body under a full Content-Length header.
func TestDownloadAbortsConnectionOnMidStreamFailure(t *testing.T) {
	handler := newHandlerWith(t, faultyBlobStore{}, nil)
	session := loginAdmin(t, handler)
	slug := createPageSlug(t, handler, session, map[string]any{"title": "DL"})

	up := performMultipart(t, handler, "/api/u/"+slug, "file", "doc.txt", []byte("data"), nil)
	if up.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", up.Code, up.Body.String())
	}

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Fatalf("expected panic(http.ErrAbortHandler), got %v", rec)
		}
	}()
	perform(t, handler, http.MethodGet, "/api/admin/pages/1/files/1", nil, session, nil)
	t.Fatal("expected download handler to abort the connection")
}
