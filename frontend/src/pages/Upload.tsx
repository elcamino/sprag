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

import { ChangeEvent, DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, Copy, FileKey2, FileUp, KeyRound, XCircle } from "lucide-react";
import { api, CreatedUpload, formatBytes, PublicPage } from "../api";
import { encryptFileForPage } from "../e2eCrypto";
import { createTransferEstimator, formatDuration } from "../transfer";

type UploadState = {
  id: string;
  name: string;
  size: number;
  loaded: number;
  total: number;
  progress: number;
  etaSeconds: number | null;
  status: "queued" | "encrypting" | "uploading" | "done" | "error";
  message?: string;
};

type SubmissionReceipt = {
  submissionID: string;
  url: string;
  copied: boolean;
};

export default function Upload() {
  const slug = useMemo(() => window.location.pathname.split("/").filter(Boolean)[1] ?? "", []);
  const [page, setPage] = useState<PublicPage | null>(null);
  const [pin, setPin] = useState("");
  const [pinUnlocked, setPinUnlocked] = useState(false);
  const [uploads, setUploads] = useState<UploadState[]>([]);
  const [receipts, setReceipts] = useState<SubmissionReceipt[]>([]);
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
    const submissionID = crypto.randomUUID();
    const accepted = files.map((file) => validateClientFile(file, page));
    const next = accepted.map(({ file, error }) => ({
      id: crypto.randomUUID(),
      name: file.name,
      size: file.size,
      loaded: 0,
      total: file.size,
      progress: error ? 0 : 1,
      etaSeconds: null,
      status: error ? "error" : "queued",
      message: error
    }) satisfies UploadState);
    setUploads((current) => [...next, ...current]);
    await Promise.all(
      accepted.map(({ file, error }, index) =>
        error ? Promise.resolve() : prepareAndUpload(file, next[index].id, submissionID)
      )
    );
  }

  async function prepareAndUpload(file: File, id: string, submissionID: string) {
    if (!page) return;
    let uploadFile = file;
    let envelope: string | undefined;
    if (page.e2e?.enabled) {
      setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "encrypting", message: "Encrypting in browser" } : item)));
      try {
        const encrypted = await encryptFileForPage(file, page.e2e);
        uploadFile = encrypted.uploadFile;
        envelope = encrypted.envelope;
      } catch (err) {
        setUploads((current) =>
          current.map((item) =>
            item.id === id ? { ...item, status: "error", message: err instanceof Error ? err.message : "Encryption failed" } : item
          )
        );
        return;
      }
    }
    await uploadFileToServer(uploadFile, id, submissionID, envelope);
  }

  function uploadFileToServer(file: File, id: string, submissionID: string, envelope?: string) {
    return new Promise<void>((resolve) => {
      const xhr = new XMLHttpRequest();
      const form = new FormData();
      const estimator = createTransferEstimator();
      form.append("submission_id", submissionID);
      if (envelope) {
        form.append("e2e_envelope", envelope);
      }
      form.append("file", file);
      xhr.open("POST", `/api/u/${slug}`);
      xhr.withCredentials = true;
      xhr.upload.onprogress = (event) => {
        if (!event.lengthComputable) return;
        const { etaSeconds } = estimator.update(event.loaded, event.total, performance.now());
        const progress = Math.max(1, Math.round((event.loaded / event.total) * 100));
        setUploads((current) =>
          current.map((item) =>
            item.id === id
              ? { ...item, status: "uploading", progress, loaded: event.loaded, total: event.total, etaSeconds }
              : item
          )
        );
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          const created = parseCreatedUpload(xhr);
          if (created?.receipt_url) {
            setReceipts((current) => upsertReceipt(current, created.submission_id, created.receipt_url));
          }
          setUploads((current) =>
            current.map((item) =>
              item.id === id ? { ...item, status: "done", progress: 100, loaded: item.total, etaSeconds: null } : item
            )
          );
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
      setUploads((current) => current.map((item) => (item.id === id ? { ...item, status: "uploading", message: undefined } : item)));
      xhr.send(form);
    });
  }

  async function copyReceipt(receipt: SubmissionReceipt) {
    await navigator.clipboard.writeText(receipt.url);
    setReceipts((current) => current.map((item) => (item.submissionID === receipt.submissionID ? { ...item, copied: true } : item)));
    window.setTimeout(() => {
      setReceipts((current) => current.map((item) => (item.submissionID === receipt.submissionID ? { ...item, copied: false } : item)));
    }, 1400);
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
    return <main className="route-loading">Sprag</main>;
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
            <p className="eyebrow">Sprag</p>
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
              {page.e2e?.enabled ? <FileKey2 size={42} /> : <FileUp size={42} />}
              <strong>Select files</strong>
              <small>
                Limit {formatBytes(page.max_size)}
                {page.allowed_ext?.length ? ` · ${page.allowed_ext.join(", ")}` : ""}
              </small>
              <input ref={inputRef} type="file" multiple onChange={chooseFiles} />
            </div>
            {receipts.length > 0 && (
              <div className="receipt-list" aria-live="polite">
                {receipts.map((receipt) => (
                  <div className="receipt-strip" key={receipt.submissionID}>
                    <span>
                      <CheckCircle2 size={18} />
                      <strong>Receipt ready</strong>
                    </span>
                    <a href={receipt.url}>{receipt.url}</a>
                    <button type="button" className="secondary-action" onClick={() => copyReceipt(receipt)}>
                      <Copy size={17} />
                      {receipt.copied ? "Copied" : "Copy"}
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="upload-list">
              {uploads.map((upload) => (
                <div className="upload-row" key={upload.id}>
                  <span>
                    <strong>{upload.name}</strong>
                    <small>{uploadDetail(upload)}</small>
                  </span>
                  <span className={`upload-status ${upload.status}`}>
                    <span className="upload-status-line">
                      {upload.status === "done" ? <CheckCircle2 size={18} /> : upload.status === "error" ? <XCircle size={18} /> : null}
                      {upload.status === "done"
                        ? "Uploaded"
                        : upload.status === "error"
                          ? "Rejected"
                          : upload.status === "encrypting"
                            ? "Encrypting"
                            : `${upload.progress}%`}
                    </span>
                    {upload.status === "uploading" && upload.etaSeconds !== null && (
                      <small className="upload-eta">{formatDuration(upload.etaSeconds)} left</small>
                    )}
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

function upsertReceipt(current: SubmissionReceipt[], submissionID: string, url: string): SubmissionReceipt[] {
  if (current.some((receipt) => receipt.submissionID === submissionID)) {
    return current.map((receipt) => (receipt.submissionID === submissionID ? { ...receipt, url } : receipt));
  }
  return [{ submissionID, url, copied: false }, ...current];
}

function parseCreatedUpload(xhr: XMLHttpRequest): CreatedUpload | null {
  try {
    return JSON.parse(xhr.responseText) as CreatedUpload;
  } catch {
    return null;
  }
}

function uploadDetail(upload: UploadState): string {
  if (upload.message) {
    return upload.message;
  }
  if (upload.status === "uploading") {
    return `${formatBytes(upload.loaded)} / ${formatBytes(upload.total)}`;
  }
  if (upload.status === "encrypting") {
    return "Encrypting before upload";
  }
  return formatBytes(upload.size);
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
