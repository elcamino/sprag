// Zener - a post-quantum-safe end-to-end encrypted file dropbox.
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

package ids_test

import (
	"regexp"
	"testing"

	"github.com/tob/zener/internal/ids"
)

func TestGenerateSlugIsBase62AndUnique(t *testing.T) {
	const count = 1000
	seen := make(map[string]bool, count)
	base62 := regexp.MustCompile(`^[A-Za-z0-9]+$`)

	for i := 0; i < count; i++ {
		slug, err := ids.GenerateSlug(24)
		if err != nil {
			t.Fatalf("GenerateSlug failed: %v", err)
		}
		if len(slug) != 24 {
			t.Fatalf("expected 24-character slug, got %q", slug)
		}
		if !base62.MatchString(slug) {
			t.Fatalf("slug contains non-base62 characters: %q", slug)
		}
		if seen[slug] {
			t.Fatalf("duplicate slug generated: %q", slug)
		}
		seen[slug] = true
	}
}

func TestNewUUIDReturnsVersion4UUID(t *testing.T) {
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatalf("NewUUID failed: %v", err)
	}
	if ok, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, id); !ok {
		t.Fatalf("expected RFC 4122 v4 UUID, got %q", id)
	}
}
