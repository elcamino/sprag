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

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elcamino/sprag/internal/ids"
	sqlite "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrDuplicateSlug is returned by CreatePage when the slug already exists.
	ErrDuplicateSlug = errors.New("duplicate slug")
)

// sqliteConstraintUnique is SQLITE_CONSTRAINT_UNIQUE, the extended result code
// for a UNIQUE constraint violation.
const sqliteConstraintUnique = 2067

func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqliteConstraintUnique
}

type SQLite struct {
	db *sql.DB
}

type Page struct {
	ID                      int64      `json:"id"`
	Slug                    string     `json:"slug"`
	Title                   string     `json:"title"`
	Description             string     `json:"description,omitempty"`
	PinHash                 string     `json:"-"`
	MaxFileSize             *int64     `json:"max_file_size,omitempty"`
	AllowedExt              string     `json:"allowed_ext,omitempty"`
	ExpiresAt               *time.Time `json:"expires_at,omitempty"`
	IsActive                bool       `json:"is_active"`
	E2EEnabled              bool       `json:"e2e_enabled"`
	E2EAlgorithm            string     `json:"e2e_algorithm,omitempty"`
	E2EPublicKey            string     `json:"e2e_public_key,omitempty"`
	E2EPublicKeyFingerprint string     `json:"e2e_public_key_fingerprint,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UploadCount             int64      `json:"upload_count"`
	TotalBytes              int64      `json:"total_bytes"`
}

type PageCreate struct {
	Slug                    string
	Title                   string
	Description             string
	PinHash                 string
	MaxFileSize             *int64
	AllowedExt              string
	ExpiresAt               *time.Time
	IsActive                bool
	E2EEnabled              bool
	E2EAlgorithm            string
	E2EPublicKey            string
	E2EPublicKeyFingerprint string
}

type NullableString struct {
	Set   bool
	Value *string
}

type NullableInt64 struct {
	Set   bool
	Value *int64
}

type NullableTime struct {
	Set   bool
	Value *time.Time
}

type PageUpdate struct {
	Title       *string
	Description NullableString
	PinHash     NullableString
	MaxFileSize NullableInt64
	AllowedExt  NullableString
	ExpiresAt   NullableTime
	IsActive    *bool
}

type Upload struct {
	ID                   int64      `json:"id"`
	PageID               int64      `json:"page_id"`
	S3Key                string     `json:"-"`
	OriginalName         string     `json:"name"`
	SizeBytes            int64      `json:"size"`
	ContentType          string     `json:"content_type,omitempty"`
	UploaderIP           string     `json:"uploader_ip,omitempty"`
	SubmissionID         string     `json:"submission_id,omitempty"`
	SubmissionUploadedAt *time.Time `json:"submission_uploaded_at,omitempty"`
	EncryptionMode       string     `json:"encryption_mode,omitempty"`
	EncryptionAlgorithm  string     `json:"encryption_algorithm,omitempty"`
	EncryptionEnvelope   string     `json:"encryption_envelope,omitempty"`
	UploadedAt           time.Time  `json:"uploaded_at"`
}

type UploadCreate struct {
	PageID              int64
	S3Key               string
	OriginalName        string
	SizeBytes           int64
	ContentType         string
	UploaderIP          string
	SubmissionID        string
	EncryptionMode      string
	EncryptionAlgorithm string
	EncryptionEnvelope  string
}

type SubmissionEnvelope struct {
	ID         int64
	PageID     int64
	PublicID   string
	UploaderIP string
	CreatedAt  time.Time
}

type SubmissionEnvelopeCreate struct {
	PageID     int64
	PublicID   string
	UploaderIP string
}

func Open(ctx context.Context, path string) (*SQLite, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	s := &SQLite{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

// dsn appends connection pragmas so every pooled connection — not just the one
// that ran migrations — is configured consistently. busy_timeout lets a writer
// wait for a contended lock instead of failing immediately with SQLITE_BUSY, and
// WAL allows reads to proceed concurrently with a writer. WAL is a persistent,
// file-level mode and is meaningless for an in-memory database, so it is omitted
// there.
func dsn(path string) string {
	pragmas := "_pragma=busy_timeout(5000)"
	if path != ":memory:" {
		pragmas += "&_pragma=journal_mode(WAL)"
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + pragmas
}

func (s *SQLite) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS pages (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT,
  pin_hash TEXT,
  max_file_size INTEGER,
  allowed_ext TEXT,
  expires_at TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  e2e_enabled INTEGER NOT NULL DEFAULT 0,
  e2e_algorithm TEXT,
  e2e_public_key TEXT,
  e2e_public_key_fingerprint TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS submission_envelopes (
  id INTEGER PRIMARY KEY,
  page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  public_id TEXT NOT NULL,
  uploader_ip TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(page_id, public_id)
);
CREATE TABLE IF NOT EXISTS uploads (
  id INTEGER PRIMARY KEY,
  page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  submission_envelope_id INTEGER REFERENCES submission_envelopes(id) ON DELETE SET NULL,
  s3_key TEXT NOT NULL,
  original_name TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  content_type TEXT,
  uploader_ip TEXT,
  encryption_mode TEXT,
  encryption_algorithm TEXT,
  encryption_envelope TEXT,
  uploaded_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_submission_envelopes_page ON submission_envelopes(page_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_uploads_page ON uploads(page_id, uploaded_at DESC);
`)
	if err != nil {
		return err
	}
	for _, column := range []struct {
		table string
		name  string
		def   string
	}{
		{"pages", "e2e_enabled", "INTEGER NOT NULL DEFAULT 0"},
		{"pages", "e2e_algorithm", "TEXT"},
		{"pages", "e2e_public_key", "TEXT"},
		{"pages", "e2e_public_key_fingerprint", "TEXT"},
		{"uploads", "submission_envelope_id", "INTEGER REFERENCES submission_envelopes(id) ON DELETE SET NULL"},
		{"uploads", "encryption_mode", "TEXT"},
		{"uploads", "encryption_algorithm", "TEXT"},
		{"uploads", "encryption_envelope", "TEXT"},
	} {
		if err := s.ensureColumn(ctx, column.table, column.name, column.def); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_uploads_submission ON uploads(submission_envelope_id, uploaded_at DESC)`); err != nil {
		return err
	}
	if err := s.backfillSubmissionEnvelopes(ctx); err != nil {
		return err
	}
	return nil
}

func (s *SQLite) ensureColumn(ctx context.Context, table, name, def string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, name, def))
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return nil
	}
	return err
}

func (s *SQLite) CreatePage(ctx context.Context, in PageCreate) (Page, error) {
	if in.Title == "" {
		return Page{}, fmt.Errorf("title is required")
	}
	if in.Slug == "" {
		return Page{}, fmt.Errorf("slug is required")
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO pages (slug, title, description, pin_hash, max_file_size, allowed_ext, expires_at, is_active,
                   e2e_enabled, e2e_algorithm, e2e_public_key, e2e_public_key_fingerprint)
VALUES (?, ?, nullif(?, ''), nullif(?, ''), ?, nullif(?, ''), ?, ?, ?, nullif(?, ''), nullif(?, ''), nullif(?, ''))`,
		in.Slug, in.Title, in.Description, in.PinHash, nullableInt(in.MaxFileSize), in.AllowedExt, formatTimePtr(in.ExpiresAt),
		1, boolInt(in.E2EEnabled), in.E2EAlgorithm, in.E2EPublicKey, in.E2EPublicKeyFingerprint)
	if isUniqueViolation(err) {
		return Page{}, ErrDuplicateSlug
	}
	if err != nil {
		return Page{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Page{}, err
	}
	return s.GetPage(ctx, id)
}

func (s *SQLite) ListPages(ctx context.Context) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.slug, p.title, coalesce(p.description, ''), coalesce(p.pin_hash, ''), p.max_file_size,
       coalesce(p.allowed_ext, ''), p.expires_at, p.is_active,
       p.e2e_enabled, coalesce(p.e2e_algorithm, ''), coalesce(p.e2e_public_key, ''), coalesce(p.e2e_public_key_fingerprint, ''),
       p.created_at,
       count(u.id), coalesce(sum(u.size_bytes), 0)
FROM pages p
LEFT JOIN uploads u ON u.page_id = p.id
GROUP BY p.id
ORDER BY p.created_at DESC, p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pages := make([]Page, 0)
	for rows.Next() {
		page, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}

func (s *SQLite) GetPage(ctx context.Context, id int64) (Page, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT p.id, p.slug, p.title, coalesce(p.description, ''), coalesce(p.pin_hash, ''), p.max_file_size,
       coalesce(p.allowed_ext, ''), p.expires_at, p.is_active,
       p.e2e_enabled, coalesce(p.e2e_algorithm, ''), coalesce(p.e2e_public_key, ''), coalesce(p.e2e_public_key_fingerprint, ''),
       p.created_at,
       count(u.id), coalesce(sum(u.size_bytes), 0)
FROM pages p
LEFT JOIN uploads u ON u.page_id = p.id
WHERE p.id = ?
GROUP BY p.id`, id)
	return scanPage(row)
}

func (s *SQLite) GetPageBySlug(ctx context.Context, slug string) (Page, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT p.id, p.slug, p.title, coalesce(p.description, ''), coalesce(p.pin_hash, ''), p.max_file_size,
       coalesce(p.allowed_ext, ''), p.expires_at, p.is_active,
       p.e2e_enabled, coalesce(p.e2e_algorithm, ''), coalesce(p.e2e_public_key, ''), coalesce(p.e2e_public_key_fingerprint, ''),
       p.created_at,
       count(u.id), coalesce(sum(u.size_bytes), 0)
FROM pages p
LEFT JOIN uploads u ON u.page_id = p.id
WHERE p.slug = ?
GROUP BY p.id`, slug)
	return scanPage(row)
}

func (s *SQLite) UpdatePage(ctx context.Context, id int64, in PageUpdate) (Page, error) {
	page, err := s.GetPage(ctx, id)
	if err != nil {
		return Page{}, err
	}
	if in.Title != nil {
		page.Title = *in.Title
	}
	if in.Description.Set {
		page.Description = valueOrEmpty(in.Description.Value)
	}
	if in.PinHash.Set {
		page.PinHash = valueOrEmpty(in.PinHash.Value)
	}
	if in.MaxFileSize.Set {
		page.MaxFileSize = in.MaxFileSize.Value
	}
	if in.AllowedExt.Set {
		page.AllowedExt = valueOrEmpty(in.AllowedExt.Value)
	}
	if in.ExpiresAt.Set {
		page.ExpiresAt = in.ExpiresAt.Value
	}
	if in.IsActive != nil {
		page.IsActive = *in.IsActive
	}
	active := 0
	if page.IsActive {
		active = 1
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE pages
SET title = ?, description = nullif(?, ''), pin_hash = nullif(?, ''), max_file_size = ?,
    allowed_ext = nullif(?, ''), expires_at = ?, is_active = ?,
    e2e_enabled = ?, e2e_algorithm = nullif(?, ''), e2e_public_key = nullif(?, ''),
    e2e_public_key_fingerprint = nullif(?, '')
WHERE id = ?`,
		page.Title, page.Description, page.PinHash, nullableInt(page.MaxFileSize), page.AllowedExt, formatTimePtr(page.ExpiresAt), active,
		boolInt(page.E2EEnabled), page.E2EAlgorithm, page.E2EPublicKey, page.E2EPublicKeyFingerprint, id)
	if err != nil {
		return Page{}, err
	}
	return s.GetPage(ctx, id)
}

func (s *SQLite) DeletePage(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pages WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) EnsureSubmissionEnvelope(ctx context.Context, in SubmissionEnvelopeCreate) (SubmissionEnvelope, error) {
	if in.PageID == 0 {
		return SubmissionEnvelope{}, fmt.Errorf("page id is required")
	}
	if in.PublicID == "" {
		return SubmissionEnvelope{}, fmt.Errorf("submission id is required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO submission_envelopes (page_id, public_id, uploader_ip)
VALUES (?, ?, nullif(?, ''))`, in.PageID, in.PublicID, in.UploaderIP)
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	return s.GetSubmissionEnvelope(ctx, in.PageID, in.PublicID)
}

func (s *SQLite) GetSubmissionEnvelope(ctx context.Context, pageID int64, publicID string) (SubmissionEnvelope, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, page_id, public_id, coalesce(uploader_ip, ''), created_at
FROM submission_envelopes
WHERE page_id = ? AND public_id = ?`, pageID, publicID)
	return scanSubmissionEnvelope(row)
}

func (s *SQLite) CreateUpload(ctx context.Context, in UploadCreate) (Upload, error) {
	submissionID := in.SubmissionID
	if strings.TrimSpace(submissionID) == "" {
		generated, err := ids.NewUUID()
		if err != nil {
			return Upload{}, err
		}
		submissionID = generated
	}
	envelope, err := s.EnsureSubmissionEnvelope(ctx, SubmissionEnvelopeCreate{
		PageID:     in.PageID,
		PublicID:   submissionID,
		UploaderIP: in.UploaderIP,
	})
	if err != nil {
		return Upload{}, err
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO uploads (page_id, submission_envelope_id, s3_key, original_name, size_bytes, content_type, uploader_ip,
                     encryption_mode, encryption_algorithm, encryption_envelope)
VALUES (?, ?, ?, ?, ?, nullif(?, ''), nullif(?, ''), nullif(?, ''), nullif(?, ''), nullif(?, ''))`,
		in.PageID, envelope.ID, in.S3Key, in.OriginalName, in.SizeBytes, in.ContentType, in.UploaderIP,
		in.EncryptionMode, in.EncryptionAlgorithm, in.EncryptionEnvelope)
	if err != nil {
		return Upload{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Upload{}, err
	}
	return s.GetUpload(ctx, in.PageID, id)
}

func (s *SQLite) ListUploads(ctx context.Context, pageID int64) ([]Upload, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT u.id, u.page_id, u.s3_key, u.original_name, u.size_bytes, coalesce(u.content_type, ''), coalesce(u.uploader_ip, ''),
       coalesce(se.public_id, ''), se.created_at,
       coalesce(u.encryption_mode, ''), coalesce(u.encryption_algorithm, ''), coalesce(u.encryption_envelope, ''),
       u.uploaded_at
FROM uploads u
LEFT JOIN submission_envelopes se ON se.id = u.submission_envelope_id
WHERE u.page_id = ?
ORDER BY coalesce(se.created_at, u.uploaded_at) DESC, se.id DESC, u.uploaded_at DESC, u.id DESC`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	uploads := make([]Upload, 0)
	for rows.Next() {
		upload, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	return uploads, rows.Err()
}

func (s *SQLite) GetUpload(ctx context.Context, pageID, uploadID int64) (Upload, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT u.id, u.page_id, u.s3_key, u.original_name, u.size_bytes, coalesce(u.content_type, ''), coalesce(u.uploader_ip, ''),
       coalesce(se.public_id, ''), se.created_at,
       coalesce(u.encryption_mode, ''), coalesce(u.encryption_algorithm, ''), coalesce(u.encryption_envelope, ''),
       u.uploaded_at
FROM uploads u
LEFT JOIN submission_envelopes se ON se.id = u.submission_envelope_id
WHERE u.page_id = ? AND u.id = ?`, pageID, uploadID)
	return scanUpload(row)
}

func (s *SQLite) DeleteUpload(ctx context.Context, pageID, uploadID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM uploads WHERE page_id = ? AND id = ?`, pageID, uploadID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPage(row scanner) (Page, error) {
	var page Page
	var max sql.NullInt64
	var expires sql.NullString
	var created string
	var active int
	var e2eEnabled int
	err := row.Scan(&page.ID, &page.Slug, &page.Title, &page.Description, &page.PinHash, &max, &page.AllowedExt, &expires, &active,
		&e2eEnabled, &page.E2EAlgorithm, &page.E2EPublicKey, &page.E2EPublicKeyFingerprint,
		&created, &page.UploadCount, &page.TotalBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, err
	}
	if max.Valid {
		page.MaxFileSize = &max.Int64
	}
	if expires.Valid && expires.String != "" {
		parsed, err := parseDBTime(expires.String)
		if err != nil {
			return Page{}, err
		}
		page.ExpiresAt = &parsed
	}
	parsed, err := parseDBTime(created)
	if err != nil {
		return Page{}, err
	}
	page.CreatedAt = parsed
	page.IsActive = active == 1
	page.E2EEnabled = e2eEnabled == 1
	return page, nil
}

func scanSubmissionEnvelope(row scanner) (SubmissionEnvelope, error) {
	var envelope SubmissionEnvelope
	var created string
	err := row.Scan(&envelope.ID, &envelope.PageID, &envelope.PublicID, &envelope.UploaderIP, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return SubmissionEnvelope{}, ErrNotFound
	}
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	parsed, err := parseDBTime(created)
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	envelope.CreatedAt = parsed
	return envelope, nil
}

func scanUpload(row scanner) (Upload, error) {
	var upload Upload
	var submissionCreated sql.NullString
	var uploaded string
	err := row.Scan(&upload.ID, &upload.PageID, &upload.S3Key, &upload.OriginalName, &upload.SizeBytes, &upload.ContentType, &upload.UploaderIP,
		&upload.SubmissionID, &submissionCreated,
		&upload.EncryptionMode, &upload.EncryptionAlgorithm, &upload.EncryptionEnvelope, &uploaded)
	if errors.Is(err, sql.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	if err != nil {
		return Upload{}, err
	}
	parsed, err := parseDBTime(uploaded)
	if err != nil {
		return Upload{}, err
	}
	if submissionCreated.Valid && submissionCreated.String != "" {
		created, err := parseDBTime(submissionCreated.String)
		if err != nil {
			return Upload{}, err
		}
		upload.SubmissionUploadedAt = &created
	}
	upload.UploadedAt = parsed
	return upload, nil
}

func (s *SQLite) backfillSubmissionEnvelopes(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO submission_envelopes (page_id, public_id, uploader_ip, created_at)
SELECT u.page_id, 'legacy-' || u.id, u.uploader_ip, u.uploaded_at
FROM uploads u
WHERE u.submission_envelope_id IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM submission_envelopes se
    WHERE se.page_id = u.page_id AND se.public_id = 'legacy-' || u.id
  );
UPDATE uploads
SET submission_envelope_id = (
  SELECT se.id
  FROM submission_envelopes se
  WHERE se.page_id = uploads.page_id AND se.public_id = 'legacy-' || uploads.id
)
WHERE submission_envelope_id IS NULL;
`)
	return err
}

func parseDBTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05.999"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid db time %q", raw)
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func nullableInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
