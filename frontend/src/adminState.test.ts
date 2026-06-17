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

import { describe, expect, it } from "vitest";
import { filesVisibleForSelectedPage, selectedPageForID } from "./adminState";
import { PageSummary, UploadFile } from "./api";

function page(id: number, title = "encryption test"): PageSummary {
  return {
    id,
    slug: `page-${id}`,
    title,
    is_active: true,
    e2e_enabled: false,
    created_at: "2026-06-17T00:00:00Z",
    upload_count: 0,
    total_bytes: 0
  };
}

function upload(id: number, pageID: number): UploadFile {
  return {
    id,
    page_id: pageID,
    name: `old-${id}.pdf`,
    size: 42,
    uploaded_at: "2026-06-17T00:00:00Z"
  };
}

describe("admin page state", () => {
  it("does not fall back to another page when an explicit selected id is missing", () => {
    expect(selectedPageForID([page(2)], 1)).toBeNull();
  });

  it("hides stale file-list responses from a deleted page", () => {
    const selected = page(2);
    const staleFiles = { pageID: 1, files: [upload(1, 1)] };

    expect(filesVisibleForSelectedPage(staleFiles, selected)).toEqual([]);
  });
});
