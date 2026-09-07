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

package filesystem_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elcamino/sprag/internal/blob"
	"github.com/elcamino/sprag/internal/filesystem"
)

func TestStoreUploadDownloadDelete(t *testing.T) {
	root := filepath.Join(t.TempDir(), "objects")
	objects, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	ctx := context.Background()
	const key = "pages/drop-id/upload-id/report.txt"

	if err := objects.Upload(ctx, key, strings.NewReader("confidential"), "text/plain"); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	stored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		t.Fatalf("read stored object: %v", err)
	}
	if string(stored) != "confidential" {
		t.Fatalf("stored object = %q, want confidential", stored)
	}

	body, err := objects.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close download: %v", err)
	}
	if string(got) != "confidential" {
		t.Fatalf("download = %q, want confidential", got)
	}

	if err := objects.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := objects.Download(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("Download after Delete = %v, want blob.ErrNotFound", err)
	}
	if err := objects.Delete(ctx, key); err != nil {
		t.Fatalf("second Delete should be idempotent, got %v", err)
	}
}

func TestStoreRejectsKeysOutsideRoot(t *testing.T) {
	root := t.TempDir()
	objects, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	for _, key := range []string{"../escape", "pages/../../escape", "/absolute", `pages\escape`} {
		t.Run(key, func(t *testing.T) {
			if err := objects.Upload(context.Background(), key, strings.NewReader("payload"), ""); err == nil {
				t.Fatalf("Upload accepted unsafe key %q", key)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe upload created an object outside the root: %v", err)
	}
}

func TestStoreRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "pages")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	objects, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	err = objects.Upload(context.Background(), "pages/escape.txt", strings.NewReader("payload"), "text/plain")
	if err == nil {
		t.Fatal("Upload followed a symlink outside the storage root")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink escape created an outside object: %v", err)
	}
}

func TestStoreDoesNotPublishPartialUpload(t *testing.T) {
	root := t.TempDir()
	objects, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	const key = "pages/drop/upload/file.txt"

	err = objects.Upload(context.Background(), key, failingReader{}, "text/plain")
	if err == nil {
		t.Fatal("Upload succeeded despite reader failure")
	}
	if _, err := objects.Download(context.Background(), key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("partial object was published: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "pages/drop/upload"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed upload left %d temporary files", len(entries))
	}
}

func TestStoreSupportsLongOriginalFilenames(t *testing.T) {
	for _, name := range []string{strings.Repeat("a", 220) + ".txt", strings.Repeat("b", 251) + ".txt", strings.Repeat("界", 80) + ".txt"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			objects, err := filesystem.New(root)
			if err != nil {
				t.Fatal(err)
			}
			key := "pages/drop/upload/" + name
			if err := objects.Upload(context.Background(), key, strings.NewReader("complete evidence"), "text/plain"); err != nil {
				t.Fatalf("upload with %d-byte filename: %v", len(name), err)
			}
			entries, err := os.ReadDir(filepath.Join(root, "pages/drop/upload"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != name {
				t.Fatalf("unexpected stored entries: %v", entries)
			}
			info, err := entries[0].Info()
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("object permissions=%v, want 0600", info.Mode().Perm())
			}
			body, err := objects.Download(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(body)
			body.Close()
			if err != nil || string(data) != "complete evidence" {
				t.Fatalf("download=%q, err=%v", data, err)
			}
		})
	}
}

func TestFailedUploadPreservesExistingObject(t *testing.T) {
	root := t.TempDir()
	objects, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "pages/drop/upload/evidence.txt"
	if err := objects.Upload(context.Background(), key, strings.NewReader("original"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if err := objects.Upload(context.Background(), key, failingReader{}, "text/plain"); err == nil {
		t.Fatal("expected write failure")
	}
	data, err := os.ReadFile(filepath.Join(root, key))
	if err != nil || string(data) != "original" {
		t.Fatalf("previous object changed: %q %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "pages/drop/upload"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("failed replacement left temporary files: %v", entries)
	}
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	copy(p, "partial")
	return len("partial"), errors.New("injected read failure")
}
