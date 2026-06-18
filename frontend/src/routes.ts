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

export type RouteKind = "home" | "upload" | "receipt" | "admin-login" | "admin-dashboard";

export function routeForPath(path: string): RouteKind {
  if (path === "/" || path === "") {
    return "home";
  }
  if (path.startsWith("/r/")) {
    return "receipt";
  }
  if (path === "/admin" || path === "/admin/") {
    return "admin-login";
  }
  if (path.startsWith("/admin")) {
    return "admin-dashboard";
  }
  return "upload";
}
