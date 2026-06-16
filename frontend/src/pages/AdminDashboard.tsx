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

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  Archive,
  Copy,
  Download,
  FileDown,
  LogOut,
  Plus,
  RefreshCw,
  Trash2,
  UploadCloud
} from "lucide-react";
import { api, CreatedPage, formatBytes, formatDate, PageSummary, UploadFile } from "../api";

type PageForm = {
  title: string;
  description: string;
  pin: string;
  max_file_size: string;
  allowed_ext: string;
  expires_at: string;
};

const emptyForm: PageForm = {
  title: "",
  description: "",
  pin: "",
  max_file_size: "",
  allowed_ext: "",
  expires_at: ""
};

export default function AdminDashboard() {
  const [pages, setPages] = useState<PageSummary[]>([]);
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [files, setFiles] = useState<UploadFile[]>([]);
  const [form, setForm] = useState<PageForm>(emptyForm);
  const [created, setCreated] = useState<CreatedPage | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const selected = useMemo(() => pages.find((page) => page.id === selectedID) ?? pages[0], [pages, selectedID]);

  const loadPages = useCallback(async () => {
    const data = await api<PageSummary[]>("/api/admin/pages");
    setPages(data);
    if (!selectedID && data.length > 0) {
      setSelectedID(data[0].id);
    }
  }, [selectedID]);

  const loadFiles = useCallback(async (pageID: number) => {
    const data = await api<UploadFile[]>(`/api/admin/pages/${pageID}/files`);
    setFiles(data);
  }, []);

  useEffect(() => {
    loadPages().catch(() => window.location.assign("/admin"));
  }, [loadPages]);

  useEffect(() => {
    if (selected) {
      loadFiles(selected.id).catch((err) => setError(err instanceof Error ? err.message : "Could not load files"));
    } else {
      setFiles([]);
    }
  }, [loadFiles, selected]);

  async function createPage(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const payload = {
        title: form.title,
        description: form.description,
        pin: form.pin,
        max_file_size: form.max_file_size ? Number(form.max_file_size) : undefined,
        allowed_ext: form.allowed_ext,
        expires_at: form.expires_at ? new Date(form.expires_at).toISOString() : ""
      };
      const page = await api<CreatedPage>("/api/admin/pages", {
        method: "POST",
        body: JSON.stringify(payload)
      });
      setCreated(page);
      setForm(emptyForm);
      await loadPages();
      setSelectedID(page.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create page");
    } finally {
      setBusy(false);
    }
  }

  async function toggleActive(page: PageSummary) {
    await api<PageSummary>(`/api/admin/pages/${page.id}`, {
      method: "PATCH",
      body: JSON.stringify({ is_active: !page.is_active })
    });
    await loadPages();
  }

  async function deletePage(page: PageSummary, filesToo: boolean) {
    if (!window.confirm(filesToo ? "Delete this page and all files?" : "Delete this page?")) return;
    await api<void>(`/api/admin/pages/${page.id}${filesToo ? "?files=1" : ""}`, { method: "DELETE" });
    setSelectedID(null);
    setCreated(null);
    await loadPages();
  }

  async function deleteFile(file: UploadFile) {
    if (!selected || !window.confirm(`Delete ${file.name}?`)) return;
    await api<void>(`/api/admin/pages/${selected.id}/files/${file.id}`, { method: "DELETE" });
    await loadFiles(selected.id);
    await loadPages();
  }

  async function logout() {
    await api("/api/admin/logout", { method: "POST" });
    window.location.assign("/admin");
  }

  return (
    <main className="admin-shell">
      <header className="admin-topbar">
        <div>
          <p className="eyebrow">Zener</p>
          <h1>Dropboxes</h1>
        </div>
        <div className="topbar-actions">
          <button className="icon-button" onClick={() => loadPages()} title="Refresh">
            <RefreshCw size={18} />
          </button>
          <button className="icon-button" onClick={logout} title="Log out">
            <LogOut size={18} />
          </button>
        </div>
      </header>

      <section className="admin-grid">
        <aside className="panel page-list-panel">
          <form onSubmit={createPage} className="new-page-form">
            <h2>
              <Plus size={18} />
              New page
            </h2>
            <label>
              <span>Title</span>
              <input value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} required />
            </label>
            <label>
              <span>Description</span>
              <textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </label>
            <div className="field-row">
              <label>
                <span>PIN</span>
                <input value={form.pin} onChange={(event) => setForm({ ...form, pin: event.target.value })} />
              </label>
              <label>
                <span>Max bytes</span>
                <input
                  value={form.max_file_size}
                  onChange={(event) => setForm({ ...form, max_file_size: event.target.value })}
                  inputMode="numeric"
                />
              </label>
            </div>
            <div className="field-row">
              <label>
                <span>Extensions</span>
                <input
                  value={form.allowed_ext}
                  onChange={(event) => setForm({ ...form, allowed_ext: event.target.value })}
                  placeholder="pdf,png,zip"
                />
              </label>
              <label>
                <span>Expires</span>
                <input
                  type="datetime-local"
                  value={form.expires_at}
                  onChange={(event) => setForm({ ...form, expires_at: event.target.value })}
                />
              </label>
            </div>
            {error && <p className="error-line">{error}</p>}
            <button className="primary-action" disabled={busy}>
              <Plus size={18} />
              <span>{busy ? "Creating" : "Create"}</span>
            </button>
          </form>

          <div className="page-list">
            {pages.map((page) => (
              <button
                key={page.id}
                className={`page-row ${selected?.id === page.id ? "selected" : ""}`}
                onClick={() => setSelectedID(page.id)}
              >
                <span className={`status-dot ${page.is_active ? "on" : "off"}`} />
                <span>
                  <strong>{page.title}</strong>
                  <small>
                    {page.upload_count} files · {formatBytes(page.total_bytes)}
                  </small>
                </span>
              </button>
            ))}
          </div>
        </aside>

        <section className="panel detail-panel">
          {selected ? (
            <>
              <div className="detail-header">
                <div>
                  <p className="eyebrow">{selected.slug}</p>
                  <h2>{selected.title}</h2>
                  {selected.description && <p className="muted">{selected.description}</p>}
                </div>
                <div className="detail-actions">
                  <button className="secondary-action" onClick={() => toggleActive(selected)}>
                    {selected.is_active ? "Deactivate" : "Activate"}
                  </button>
                  <button className="icon-button danger" onClick={() => deletePage(selected, false)} title="Delete page">
                    <Trash2 size={18} />
                  </button>
                  <button className="icon-button danger" onClick={() => deletePage(selected, true)} title="Delete page and files">
                    <Archive size={18} />
                  </button>
                </div>
              </div>

              <ShareBlock page={created?.id === selected.id ? created : null} fallbackSlug={selected.slug} />

              <div className="file-toolbar">
                <h3>
                  <UploadCloud size={18} />
                  Files
                </h3>
                <a className="secondary-action" href={`/api/admin/pages/${selected.id}/zip`}>
                  <FileDown size={17} />
                  Zip
                </a>
              </div>

              <div className="file-table">
                {files.map((file) => (
                  <div className="file-row" key={file.id}>
                    <span>
                      <strong>{file.name}</strong>
                      <small>
                        {formatBytes(file.size)} · {formatDate(file.uploaded_at)}
                      </small>
                    </span>
                    <span className="file-actions">
                      <a className="icon-button" href={`/api/admin/pages/${selected.id}/files/${file.id}`} title="Download">
                        <Download size={17} />
                      </a>
                      <button className="icon-button danger" onClick={() => deleteFile(file)} title="Delete file">
                        <Trash2 size={17} />
                      </button>
                    </span>
                  </div>
                ))}
                {files.length === 0 && <div className="empty-state">No files yet</div>}
              </div>
            </>
          ) : (
            <div className="empty-state">No pages yet</div>
          )}
        </section>
      </section>
    </main>
  );
}

function ShareBlock({ page, fallbackSlug }: { page: CreatedPage | null; fallbackSlug: string }) {
  const url = page?.url ?? `${window.location.origin}/u/${fallbackSlug}`;
  const [copied, setCopied] = useState(false);

  async function copy() {
    await navigator.clipboard.writeText(url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  }

  return (
    <div className="share-block">
      <QRCodeSVG value={url} size={112} marginSize={1} />
      <div>
        <p className="eyebrow">Share URL</p>
        <code>{url}</code>
        <button className="secondary-action" onClick={copy}>
          <Copy size={17} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}
