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

package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/elcamino/sprag/internal/blob"
)

type Store struct {
	rootPath string
}

func New(rootPath string) (*Store, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return nil, fmt.Errorf("local storage path is required")
	}
	if err := os.MkdirAll(rootPath, 0o700); err != nil {
		return nil, fmt.Errorf("create local storage root: %w", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open local storage root: %w", err)
	}
	if err := root.Close(); err != nil {
		return nil, fmt.Errorf("close local storage root: %w", err)
	}
	return &Store{rootPath: rootPath}, nil
}

func (s *Store) Upload(ctx context.Context, key string, body io.Reader, _ string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return fmt.Errorf("open local storage root: %w", err)
	}
	defer root.Close()

	dir := path.Dir(key)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}
	tempKey, err := temporaryKey(dir)
	if err != nil {
		return err
	}
	temp, err := root.OpenFile(tempKey, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary object: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = root.Remove(tempKey)
		}
	}()

	if _, err := io.Copy(temp, contextReader{ctx: ctx, reader: body}); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write object: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync object: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close object: %w", err)
	}
	if err := root.Rename(tempKey, key); err != nil {
		return fmt.Errorf("publish object: %w", err)
	}
	committed = true
	return nil
}

func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("open local storage root: %w", err)
	}
	object, err := root.Open(key)
	closeErr := root.Close()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, blob.ErrNotFound
		}
		return nil, fmt.Errorf("open object: %w", err)
	}
	if closeErr != nil {
		_ = object.Close()
		return nil, fmt.Errorf("close local storage root: %w", closeErr)
	}
	return object, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return fmt.Errorf("open local storage root: %w", err)
	}
	defer root.Close()
	if err := root.Remove(key); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete object: %w", err)
	}
	// Each upload has its own UUID directory, so remove that directory when it
	// becomes empty. Keep shared slug and prefix directories in place to avoid
	// racing a concurrent upload that is about to create a sibling object.
	if dir := path.Dir(key); dir != "." {
		_ = root.Remove(dir)
	}
	return nil
}

func validateKey(key string) error {
	if !fs.ValidPath(key) || strings.ContainsRune(key, '\\') || strings.IndexByte(key, 0) >= 0 {
		return fmt.Errorf("invalid object key %q", key)
	}
	return nil
}

func temporaryKey(dir string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary object name: %w", err)
	}
	// A valid final filename may already be at the filesystem's component
	// limit. Keep the temporary name short and independent of the original.
	return path.Join(dir, ".sprag-upload-"+hex.EncodeToString(random[:])+".tmp"), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

var _ blob.Store = (*Store)(nil)
