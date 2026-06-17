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
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func FS(fsys fs.FS) http.FileSystem {
	return http.FS(fsys)
}

func spaHandler(staticFS http.FileSystem) http.HandlerFunc {
	fileServer := http.FileServer(staticFS)
	return func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || strings.HasPrefix(clean, "admin") || strings.HasPrefix(clean, "u/") {
			serveIndex(w, r, staticFS)
			return
		}
		if f, err := staticFS.Open(clean); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, staticFS)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request, staticFS http.FileSystem) {
	file, err := staticFS.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, file)
}
