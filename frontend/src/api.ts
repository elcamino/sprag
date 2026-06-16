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

export type ApiError = {
  error: {
    code: string;
    message: string;
  };
};

export type PageSummary = {
  id: number;
  slug: string;
  title: string;
  description?: string;
  max_file_size?: number;
  allowed_ext?: string;
  expires_at?: string;
  is_active: boolean;
  created_at: string;
  upload_count: number;
  total_bytes: number;
};

export type UploadFile = {
  id: number;
  page_id: number;
  name: string;
  size: number;
  content_type?: string;
  uploader_ip?: string;
  uploaded_at: string;
};

export type PublicPage = {
  title: string;
  description?: string;
  pin_required: boolean;
  max_size: number;
  allowed_ext?: string[];
};

export type CreatedPage = {
  id: number;
  slug: string;
  url: string;
  title: string;
  description?: string;
};

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = init.method ?? "GET";
  const headers = new Headers(init.headers);
  if (method !== "GET" && method !== "HEAD") {
    headers.set("X-Zener-CSRF", "1");
  }
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, {
    ...init,
    method,
    headers,
    credentials: "include"
  });
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    let code = "request_failed";
    try {
      const data = (await response.json()) as ApiError;
      message = data.error.message;
      code = data.error.code;
    } catch {
      // Keep the HTTP fallback.
    }
    throw Object.assign(new Error(message), { code, status: response.status });
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let size = value / 1024;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[index]}`;
}

export function formatDate(value?: string): string {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(new Date(value));
}
