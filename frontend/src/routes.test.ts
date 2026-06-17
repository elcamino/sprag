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

import { describe, expect, it } from "vitest";
import { routeForPath } from "./routes";

describe("routeForPath", () => {
  it("routes exact root to the home page", () => {
    expect(routeForPath("/")).toBe("home");
  });

  it("keeps public upload pages on the upload route", () => {
    expect(routeForPath("/u/abc123")).toBe("upload");
  });

  it("routes bare admin to the login page", () => {
    expect(routeForPath("/admin")).toBe("admin-login");
    expect(routeForPath("/admin/")).toBe("admin-login");
  });

  it("routes admin child pages to the dashboard", () => {
    expect(routeForPath("/admin/pages")).toBe("admin-dashboard");
  });
});
