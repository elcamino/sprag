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

import { ChangeEvent, DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, FileUp, KeyRound, XCircle } from "lucide-react";
import { api, formatBytes, PublicPage } from "../api";

type UploadState = {
  id: string;
  name: string;
  size: number;
  progress: number;
  status: "queued" | "uploading" | "done" | "error";
  message?: string;
};

export default function Upload() {
  const slug = useMemo(() => window.location.pathname.split("/").filter(Boolean)[1] ?? "", []);
  const [page, setPage] = useState<PublicPage | null>(null);
  const [pin, setPin] = useState("");
  const [pinUnlocked, setPinUnlocked] = useState(false);
  const [uploads, setUploads] = useState<UploadState[]>([]);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api<PublicPage>(`/api/u/${slug}`)
      .then(setPage)
      .catch((err) => setError(err instanceof Error ? err.message : "This page is closed"));
  }, [slug]);

  async function submitPin(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await api(`/api/u/${slug}/pin`, {
        method: "POST",
        body: JSON.stringify({ pin })
      });
      setPinUnlocked(true);
      setPin("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "PIN failed");
    }
  }

  function chooseFiles(event: ChangeEvent<HTMLInputElement>) {
    if (event.target.files) {
      void enqueue(Array.from(event.target.files));
      event.target.value = "";
    }
  }

  function drop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    void enqueue(Array.from(event.dataTransfer.files));
  }

  async function enqueue(files: File[]) {
    if (!page) return;
    const accepted = files.map((file) => validateClientFile(file, page));
    const next = accepted.map(({ file, error }) => ({
      id: crypto.randomUUID(),
      name: file.name,
      size: file.size,
      progress: error ? 0 : 1,
      status: error ? "error" : "queued",
      message: error
    }) satisfies UploadState);
    setUploads((current) => [...next, ...current]);
    await Promise.all(
      accepted.map(({ file, error }, index) => (error ? Promise.resolve() : uploadFile(file, next[index].id)))
    );
  }

  function uploadFile(file: File, id: string) {
    return new Promise<void>((resolve) => {
      const xhr = new XMLHttpRequest();
      const form = new FormData();
      form.append("file", file);
      xhr.open("POST", `/api/u/${slug}`);
      xhr.withCredentials = true;
      xhr.upload.onprogress = (event) => {
        if (!event.lengthComputable) return;
        const progress = Math.max(1, Math.round((event.loaded / event.total) * 100));
        setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "uploading", progress } : item)));
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "done", progress: 100 } : item)));
        } else {
          setUploads((current) =>
            current.map((item) => (item.id === id ? { ...item, status: "error", message: parseXHRMessage(xhr) } : item))
          );
        }
        resolve();
      };
      xhr.onerror = () => {
        setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "error", message: "Network error" } : item)));
        resolve();
      };
      setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "uploading" } : item)));
      xhr.send(form);
    });
  }

  if (error && !page) {
    return (
      <main className="upload-shell">
        <section className="closed-panel">
          <XCircle size={28} />
          <h1>{error}</h1>
        </section>
      </main>
    );
  }

  if (!page) {
    return <main className="route-loading">Zener</main>;
  }

  const locked = page.pin_required && !pinUnlocked;

  return (
    <main className="upload-shell">
      <section className="drop-panel">
        <div className="upload-heading">
          <span className="mark">
            <FileUp size={22} />
          </span>
          <div>
            <p className="eyebrow">Zener</p>
            <h1>{page.title}</h1>
            {page.description && <p>{page.description}</p>}
          </div>
        </div>

        {locked ? (
          <form onSubmit={submitPin} className="pin-form">
            <label>
              <span>PIN</span>
              <input value={pin} onChange={(event) => setPin(event.target.value)} autoFocus />
            </label>
            {error && <p className="error-line">{error}</p>}
            <button className="primary-action">
              <KeyRound size={18} />
              Unlock
            </button>
          </form>
        ) : (
          <>
            <div
              className="drop-zone"
              onDragOver={(event) => event.preventDefault()}
              onDrop={drop}
              onClick={() => inputRef.current?.click()}
            >
              <FileUp size={42} />
              <strong>Select files</strong>
              <small>
                Limit {formatBytes(page.max_size)}
                {page.allowed_ext?.length ? ` · ${page.allowed_ext.join(", ")}` : ""}
              </small>
              <input ref={inputRef} type="file" multiple onChange={chooseFiles} />
            </div>
            <div className="upload-list">
              {uploads.map((upload) => (
                <div className="upload-row" key={upload.id}>
                  <span>
                    <strong>{upload.name}</strong>
                    <small>{upload.message ?? formatBytes(upload.size)}</small>
                  </span>
                  <span className={`upload-status ${upload.status}`}>
                    {upload.status === "done" ? <CheckCircle2 size={18} /> : upload.status === "error" ? <XCircle size={18} /> : null}
                    {upload.status === "done" ? "Uploaded" : upload.status === "error" ? "Rejected" : `${upload.progress}%`}
                  </span>
                  <span className="progress-track">
                    <span style={{ width: `${upload.progress}%` }} />
                  </span>
                </div>
              ))}
            </div>
          </>
        )}
      </section>
    </main>
  );
}

function validateClientFile(file: File, page: PublicPage): { file: File; error?: string } {
  if (file.size > page.max_size) {
    return { file, error: `Over ${formatBytes(page.max_size)}` };
  }
  if (page.allowed_ext?.length) {
    const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
    if (!page.allowed_ext.includes(ext)) {
      return { file, error: "Extension not allowed" };
    }
  }
  return { file };
}

function parseXHRMessage(xhr: XMLHttpRequest): string {
  try {
    return JSON.parse(xhr.responseText).error.message;
  } catch {
    return `${xhr.status} ${xhr.statusText}`;
  }
}
