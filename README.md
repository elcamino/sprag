# Zener

**A tiny, self-hostable, server-blind secure intake box.**
One Go binary. Anonymous uploads. Post-quantum end-to-end encryption. Nothing flows back out.

---

Zener is named after the [Zener diode](https://en.wikipedia.org/wiki/Zener_diode): data flows **one way only**. Uploaders push files into an unguessable upload-page URL — they can never list, download, or even see what else has arrived. Only the authenticated admin can read what came in.

It is **not** a file-sharing product. It is an **asymmetric, anonymous intake box**: the admin creates a capability URL, hands it out, and someone on the other side drops files in. That is the whole shape of the product, and everything else is built to keep that shape small and legible.

With **server-blind E2E intake** enabled, the uploader's browser encrypts every file with **post-quantum hybrid cryptography** *before a single byte leaves the device*. The Go server and your S3 bucket only ever touch ciphertext. The admin decrypts client-side at download time. There is no plaintext for the server — or anyone who compromises it — to read.

> A SecureDrop-grade intake capability without SecureDrop's operational weight: no Tor, no hardened workstation, no multi-server deployment. Just one binary behind your existing reverse proxy.

## Who it's for

- **Lawyers** receiving privileged or sensitive client documents
- **Journalists** receiving source material and leaks
- **HR / compliance teams** running whistleblower channels
- **Doctors and researchers** collecting sensitive records
- **Anyone** who needs to collect files from people who should not need an account

## Why Zener

- **One-way by construction.** The uploader API surface is exactly three routes. There is no listing endpoint a sender can reach. Knowing one page's URL reveals nothing about any other page or the admin area.
- **No accounts for uploaders. Ever.** The unguessable URL *is* the capability. That same trusted channel also carries the page's public key, so server-blind encryption needs no separate PKI or key-exchange ceremony.
- **Server-blind, post-quantum E2E.** Optional per-deployment, optional or required per-page. ML-KEM-1024 + P-384 hybrid KEM, HKDF-SHA-512, AES-256-GCM — encrypted in the browser before upload.
- **Tiny and legible.** A single CGO-free Go binary with an embedded React frontend, one `.env`, one SQLite file, one S3 bucket. You can read the whole threat model in an afternoon.
- **Bounded memory at any file size.** Uploads stream straight into an S3 multipart upload and downloads stream straight back out. A 5 GB file never lands on local disk or fills RAM.

## How it compares

Most "send me a file" tools are **outbound** sharing products retrofitted for inbound use, and their servers can read your files in normal operation. Zener is built the other way around: inbound-only intake is the *only* thing it does, which is exactly why server blindness fits naturally instead of being bolted on.

| | **Zener** | WeTransfer / Dropbox Transfer | Nextcloud File Drop | Google Forms / Typeform upload | SecureDrop |
|---|:---:|:---:|:---:|:---:|:---:|
| Primary direction | **Inbound intake** | Outbound sharing | Two-way sync/share | Form responses | Inbound intake |
| Uploader needs an account | **No** | No | Sometimes | Often (Google) | No |
| Uploader can list/retrieve others' files | **No, by design** | N/A | Configurable | No | No |
| Anonymous uploads | **Yes** | Yes | Yes | Limited | Yes (Tor) |
| End-to-end / server-blind | **Yes (optional)** | No | No | No | Yes |
| **Post-quantum** encryption | **Yes** | No | No | No | No |
| Self-hostable | **Yes** | No | Yes | No | Yes |
| Deployment footprint | **One binary** | SaaS | Heavy PHP stack | SaaS | Multi-server + Tor + hardened workstation |
| Per-page capability URL | **Yes** | Per-transfer link | Per-share link | Per-form link | Per-source codename |
| Large files (multi-GB, streamed) | **Yes** | Yes (paid tiers) | Yes | No | Limited |
| Storage you control | **Any S3-compatible** | Vendor | Yours | Vendor | Yours |

Zener deliberately does **not** try to be a Dropbox, a ticketing system, or a form builder. There are no folders, no comment threads, no multi-tenant sharing permissions. That restraint is the moat.

## Features

- **Unguessable upload pages** — 24-character base62 slugs from `crypto/rand`. A page has a title, optional description, optional PIN, optional expiry, optional per-page max file size, an optional allow-list of extensions, and an active flag.
- **Drag-and-drop uploads** with multi-file support and per-file progress (bytes + ETA).
- **Optional PIN** per page (bcrypt-hashed, rate-limited per slug+IP).
- **Admin dashboard** — create/edit/delete pages, list uploads with name/size/time, download a single file, or download a whole page as a streamed `.zip`.
- **QR codes and copy buttons** for sharing capability URLs.
- **Server-blind post-quantum E2E intake** (see below).
- **Single static binary** with the frontend embedded via `embed.FS`. Pure-Go SQLite means CGO-free builds and trivial cross-compilation.

## Security model

- **No uploader-reachable listing.** The public surface is exactly `GET /api/u/:slug` (metadata), `POST /api/u/:slug/pin`, and `POST /api/u/:slug`. Upload responses never include other files.
- **Unguessable capability URLs.** ≥ 24 chars, base62, cryptographically random.
- **Admin password** hashed with bcrypt. Supply it as plaintext (`ADMIN_PASSWORD`) or — better — as a precomputed bcrypt hash (`ADMIN_PASSWORD_HASH`) so the plaintext never lives in your config. Passwords beyond bcrypt's 72-byte limit are handled via an internal SHA-256 prehash.
- **Rate limiting.** Admin login 5/min/IP, PIN attempts 10/min per slug+IP, keyed on the real client IP (see `TRUSTED_PROXY_HOPS`).
- **Sessions.** Stateless HMAC-signed cookies, 7-day expiry, `HttpOnly` + `Secure` + `SameSite=Lax`.
- **CSRF.** Admin mutations require the `X-Zener-CSRF` custom header in addition to the same-site cookie.
- **Streaming with hard caps.** The size limit is enforced by a counting reader while streaming; an oversized upload aborts the S3 multipart upload instead of trusting `Content-Length`. Files are never buffered whole in memory or on disk.
- **Path-safe storage.** S3 keys use server-generated UUID paths (`S3_PREFIX/<slug>/<uuid>/<filename>`); the original filename is metadata only, so a malicious name can't traverse or collide.
- **Downloads are always `Content-Disposition: attachment`**, never inline, so the bucket can't be used as an XSS host.
- **Secrets are never logged.** Startup echoes a redacted config.

## Quick start (local)

**Prerequisites:** Go ≥ 1.26, Node ≥ 22, and an S3-compatible bucket (Wasabi, Backblaze B2, MinIO, AWS S3, …).

1. **Copy the example config:**

   ```bash
   cp .env.example .env
   ```

2. **Generate a session secret** (≥ 32 bytes, base64) and put it in `.env`:

   ```bash
   openssl rand -base64 32
   ```

3. **Set the admin password.** Either set `ADMIN_PASSWORD` directly, or — recommended — store only a bcrypt hash so the plaintext never lives in your config:

   ```bash
   go run ./cmd/zener hash-password            # prompts for the password
   go run ./cmd/zener hash-password 'your-pw'  # or pass it as an argument
   ```

   Put the printed hash in `ADMIN_PASSWORD_HASH`. (If both are set, the hash wins.)

4. **Fill in the S3 settings** (`S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`) and `BASE_URL`.

5. **Build the frontend and run the server:**

   ```bash
   cd frontend
   npm install
   npm run build
   cd ..
   go run ./cmd/zener
   ```

The admin UI is at `/admin`. Public upload pages are created from the admin dashboard. Startup fails fast with a clear message if any required secret or S3 value is missing.

## Docker / production

The bundled `docker-compose.yml` is a self-contained demo: it builds the app (multi-stage → distroless static image) and runs a Caddy reverse proxy in front of it.

```bash
cp .env.example .env   # fill in the secrets and S3 settings
ZENER_DOMAIN=localhost docker compose up --build
```

Caddy listens on ports 80/443 and forwards to `zener-app:8080`. Set `ZENER_DOMAIN` to a real domain (with DNS pointing at the host) to get **automatic HTTPS**; it defaults to `localhost`, for which Caddy issues a local CA certificate.

Because Caddy is the single proxy in front of the app, keep `TRUSTED_PROXY_HOPS=1` in `.env`. Zener then reads the client address Caddy appends to `X-Forwarded-For` and ignores any spoofed value a client puts on the left, so login and PIN rate limiting key on the real client IP.

### Using an existing shared Caddy instead

If you already run a shared Caddy on an external network, drop the `caddy` service and attach the app to that network instead. Use a unique service name (`zener-app`) to avoid a DNS-alias collision with another project's container, and add a block to your existing Caddyfile:

```caddyfile
zener.example.com {
    reverse_proxy zener-app:8080
}
```

Keep `TRUSTED_PROXY_HOPS` equal to the number of proxies that append to `X-Forwarded-For` in front of the app (1 for a single Caddy). If you expose the app directly with no proxy, set `TRUSTED_PROXY_HOPS=0` so the header is ignored and the real TCP peer is used.

## Configuration

Zener loads `.env` if present and then reads environment variables. Startup fails fast if required secrets or S3 values are missing.

| Variable | Required | Default | Notes |
|---|:---:|---|---|
| `PORT` | | `8080` | Listen port. |
| `BASE_URL` | ✓ | | Used to build shareable `/u/<slug>` URLs. |
| `SESSION_SECRET` | ✓ | | Base64; must decode to ≥ 32 bytes. Rotating it invalidates all sessions. |
| `ADMIN_USERNAME` | | `admin` | |
| `ADMIN_PASSWORD` | ✓\* | | Plaintext, bcrypt-hashed in memory at boot. |
| `ADMIN_PASSWORD_HASH` | ✓\* | | Precomputed bcrypt hash (preferred). Takes precedence over `ADMIN_PASSWORD`. |
| `MAX_FILE_SIZE` | | `5368709120` (5 GiB) | Global default; per-page limits may only lower it. |
| `ALLOWED_EXT` | | *(any)* | Comma list, e.g. `pdf,png,zip`. A hard ceiling per-page lists may narrow but not widen. |
| `TRUSTED_PROXY_HOPS` | | `1` | Number of trusted proxies appending to `X-Forwarded-For`. `0` = directly exposed. |
| `DB_PATH` | | `/data/zener.db` | SQLite path (WAL mode; back up the whole directory). |
| `S3_ENDPOINT` | ✓ | | S3-compatible endpoint. |
| `S3_REGION` | ✓ | | |
| `S3_BUCKET` | ✓ | | |
| `S3_ACCESS_KEY` | ✓ | | |
| `S3_SECRET_KEY` | ✓ | | |
| `S3_USE_PATH_STYLE` | | `false` | `true` for MinIO. |
| `S3_PREFIX` | | `pages/` | Key namespace inside the bucket. |
| `E2E_INTAKE_ENABLED` | | `false` | Enables server-blind E2E intake. |
| `E2E_INTAKE_REQUIRED` | | `false` | Rejects plaintext pages/uploads. Requires `E2E_INTAKE_ENABLED=true`. |
| `E2E_INTAKE_ALGORITHM` | | `ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM` | The only supported profile. |

\* Exactly one of `ADMIN_PASSWORD` / `ADMIN_PASSWORD_HASH` is required.

`MAX_FILE_SIZE` defaults to 5 GiB. Admin sessions are stateless signed cookies (7-day expiry). Rotating `SESSION_SECRET` invalidates every outstanding session immediately; changing only the admin password does not, so rotate the secret too if you need to force existing sessions to log out.

## Server-blind post-quantum E2E intake

When `E2E_INTAKE_ENABLED=true`, admins can create pages whose uploads are encrypted in the uploader's browser **before any bytes leave the device**. Set `E2E_INTAKE_REQUIRED=true` to reject plaintext pages and plaintext uploads entirely.

**How it works:**

1. The admin generates an encryption identity in the browser. The **public key** is attached to the upload page; the **private key** never touches the server.
2. The public key rides on the same unguessable capability URL the admin already shares — no separate PKI or key server.
3. The uploader's browser encrypts each file and its metadata locally and uploads only ciphertext.
4. The server stores page public keys and encrypted upload envelopes, but **never** stores E2E private keys.
5. The admin decrypts downloads client-side with the private key.

**The cryptographic profile** is `ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM`:
the browser combines **ML-KEM-1024** (post-quantum KEM, via [`@noble/post-quantum`](https://github.com/paulmillr/noble-post-quantum)) with **P-384 ECDH** in a hybrid construction, derives **AES-256-GCM** keys with **HKDF-SHA-512**, encrypts file bytes and metadata locally, and uploads only ciphertext. The hybrid design means an attacker would have to break *both* a classical and a post-quantum primitive — and recorded ciphertext stays safe against future quantum "harvest now, decrypt later" attacks.

> ⚠️ **Back up each generated private key.** If it is lost, the matching encrypted uploads **cannot be recovered** — not by you, not by anyone. The server is blind by design.

The admin UI can optionally store a generated private key encrypted in browser IndexedDB. The stored value is encrypted with a passphrase that the admin supplies and that Zener does not save. This is safer than keeping an unencrypted downloaded key, but weaker than a password manager or offline backup, and still does not protect against a compromised admin-origin script after the key is unlocked.

## Storage

Metadata is stored in SQLite at `DB_PATH`. The database runs in WAL mode with a busy timeout so concurrent uploads don't fail under lock contention; this creates `-wal` and `-shm` sidecar files next to `DB_PATH`, so back up the whole directory. File bodies are streamed to S3-compatible object storage under `S3_PREFIX/<slug>/<uuid>/<filename>`.

## Tech stack

- **Backend:** Go (stdlib `net/http` + `chi`), pure-Go SQLite (`modernc.org/sqlite`, CGO-free), AWS SDK v2 for any S3-compatible endpoint, `log/slog` JSON logging.
- **Frontend:** React + TypeScript + Vite + Tailwind CSS, built to `frontend/dist/` and embedded into the binary.
- **Crypto:** bcrypt for passwords/PINs; `@noble/post-quantum` (ML-KEM-1024) + WebCrypto (P-384 ECDH, HKDF-SHA-512, AES-256-GCM) for E2E.
- **Deployment:** multi-stage Dockerfile → `gcr.io/distroless/static`, `docker-compose.yml` with Caddy for automatic HTTPS.

## License

Zener is free software under the **GNU General Public License v3.0** (or later). See [LICENSE.md](LICENSE.md).
