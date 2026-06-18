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

import { PageSummary, ReceiptStatus, UploadFile } from "./api";

export type LoadedFiles = {
  pageID: number;
  files: UploadFile[];
} | null;

export type PrivateKeyControlState = "unlock" | "remove-memory";

export type DownloadUnlockPrompt = {
  pageID: number;
  nonce: number;
};

export type SubmissionFileGroup = {
  submissionID: string;
  uploadedAt: string;
  receiptToken?: string;
  receiptStatus?: ReceiptStatus;
  receiptStatusUpdatedAt?: string;
  files: UploadFile[];
  fileCount: number;
  totalBytes: number;
};

export function selectedPageForID(pages: PageSummary[], selectedID: number | null): PageSummary | null {
  if (selectedID === null) {
    return pages[0] ?? null;
  }
  return pages.find((page) => page.id === selectedID) ?? null;
}

export function filesVisibleForSelectedPage(loadedFiles: LoadedFiles, selected: PageSummary | null): UploadFile[] {
  if (!selected || !loadedFiles || loadedFiles.pageID !== selected.id) {
    return [];
  }
  return loadedFiles.files;
}

export function groupFilesBySubmission(files: UploadFile[]): SubmissionFileGroup[] {
  const groups: SubmissionFileGroup[] = [];
  const bySubmission = new Map<string, SubmissionFileGroup>();
  for (const file of files) {
    const submissionID = file.submission_id || `file-${file.id}`;
    let group = bySubmission.get(submissionID);
    if (!group) {
      group = {
        submissionID,
        uploadedAt: file.submission_uploaded_at || file.uploaded_at,
        receiptToken: file.receipt_token,
        receiptStatus: file.receipt_status,
        receiptStatusUpdatedAt: file.receipt_status_updated_at,
        files: [],
        fileCount: 0,
        totalBytes: 0
      };
      bySubmission.set(submissionID, group);
      groups.push(group);
    }
    group.files.push(file);
    group.fileCount += 1;
    group.totalBytes += file.size;
  }
  return groups;
}

export function privateKeyControlState(loadedPrivateKey?: string): PrivateKeyControlState {
  return loadedPrivateKey?.trim() ? "remove-memory" : "unlock";
}

export function submitStoredPrivateKeyUnlock<T>(
  event: { preventDefault(): void },
  page: T,
  unlock: (page: T) => void | Promise<void>
): void {
  event.preventDefault();
  void unlock(page);
}

export function nextDownloadUnlockPrompt(
  current: DownloadUnlockPrompt | null,
  pageID: number,
  loadedPrivateKey?: string
): DownloadUnlockPrompt | null {
  if (loadedPrivateKey?.trim()) {
    return null;
  }
  return {
    pageID,
    nonce: (current?.nonce ?? 0) + 1
  };
}

export function downloadUnlockPromptActive(prompt: DownloadUnlockPrompt | null, pageID: number): boolean {
  return prompt?.pageID === pageID;
}
