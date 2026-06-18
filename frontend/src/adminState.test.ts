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
import {
  downloadUnlockPromptActive,
  filesVisibleForSelectedPage,
  groupFilesBySubmission,
  nextDownloadUnlockPrompt,
  privateKeyControlState,
  receiptStatusHelp,
  receiptStatusLabel,
  sealActionHelp,
  selectedPageForID,
  submitStoredPrivateKeyUnlock
} from "./adminState";
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

  it("shows the remove-memory control when a private key is already loaded", () => {
    expect(privateKeyControlState("  private key JSON  ")).toBe("remove-memory");
  });

  it("shows the unlock control when a private key is not loaded", () => {
    expect(privateKeyControlState("")).toBe("unlock");
    expect(privateKeyControlState(undefined)).toBe("unlock");
  });

  it("submits stored-key unlock without navigating away", () => {
    const selected = page(2);
    let defaultPrevented = false;
    let unlockedPage: PageSummary | null = null;

    submitStoredPrivateKeyUnlock(
      {
        preventDefault: () => {
          defaultPrevented = true;
        }
      },
      selected,
      (page) => {
        unlockedPage = page;
      }
    );

    expect(defaultPrevented).toBe(true);
    expect(unlockedPage).toBe(selected);
  });

  it("prompts unlock for encrypted downloads when no private key is loaded", () => {
    const firstPrompt = nextDownloadUnlockPrompt(null, 2, "");
    const secondPrompt = nextDownloadUnlockPrompt(firstPrompt, 2, "  ");

    expect(firstPrompt).toEqual({ pageID: 2, nonce: 1 });
    expect(secondPrompt).toEqual({ pageID: 2, nonce: 2 });
    expect(downloadUnlockPromptActive(secondPrompt, 2)).toBe(true);
    expect(downloadUnlockPromptActive(secondPrompt, 3)).toBe(false);
  });

  it("does not prompt unlock for encrypted downloads when a private key is loaded", () => {
    expect(nextDownloadUnlockPrompt(null, 2, "private key JSON")).toBeNull();
  });

  it("keeps files from one submission envelope together", () => {
    const groups = groupFilesBySubmission([
      {
        id: 2,
        page_id: 1,
        name: "two.txt",
        size: 2,
        uploaded_at: "2026-06-17T10:00:02Z",
        submission_id: "submission-a",
        submission_uploaded_at: "2026-06-17T10:00:00Z",
        receipt_token: "receipt-a",
        receipt_status: "received",
        receipt_status_updated_at: "2026-06-17T10:00:03Z"
      },
      {
        id: 1,
        page_id: 1,
        name: "one.txt",
        size: 1,
        uploaded_at: "2026-06-17T10:00:01Z",
        submission_id: "submission-a",
        submission_uploaded_at: "2026-06-17T10:00:00Z",
        receipt_token: "receipt-a",
        receipt_status: "received",
        receipt_status_updated_at: "2026-06-17T10:00:03Z"
      },
      {
        id: 3,
        page_id: 1,
        name: "single.txt",
        size: 3,
        uploaded_at: "2026-06-17T09:00:00Z",
        submission_id: "submission-b",
        submission_uploaded_at: "2026-06-17T09:00:00Z",
        receipt_token: "receipt-b",
        receipt_status: "reviewed",
        receipt_status_updated_at: "2026-06-17T09:30:00Z"
      }
    ]);

    expect(groups).toHaveLength(2);
    expect(groups[0]).toMatchObject({
      submissionID: "submission-a",
      fileCount: 2,
      totalBytes: 3,
      uploadedAt: "2026-06-17T10:00:00Z",
      receiptToken: "receipt-a",
      receiptStatus: "received",
      receiptStatusUpdatedAt: "2026-06-17T10:00:03Z"
    });
    expect(groups[0].files.map((file) => file.name)).toEqual(["two.txt", "one.txt"]);
    expect(groups[1]).toMatchObject({
      submissionID: "submission-b",
      fileCount: 1,
      totalBytes: 3,
      receiptToken: "receipt-b",
      receiptStatus: "reviewed"
    });
  });

  it("describes receipt status with compact label and tooltip copy", () => {
    expect(receiptStatusLabel).toBe("File status");
    expect(receiptStatusHelp).toContain("receipt link");
    expect(receiptStatusHelp).toContain("does not grant file access");
  });

  it("describes what sealing a page does", () => {
    expect(sealActionHelp).toContain("closes public uploads");
    expect(sealActionHelp).toContain("prevents reopening");
    expect(sealActionHelp).toContain("custody log");
  });
});
