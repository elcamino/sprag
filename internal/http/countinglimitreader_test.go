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

package httpapi

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCountingLimitReaderUnderLimitReadsEverything(t *testing.T) {
	src := bytes.NewReader([]byte("hello"))
	r := &countingLimitReader{r: src, remaining: 100}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll error = %v, want nil", err)
	}
	if string(got) != "hello" {
		t.Fatalf("read %q, want hello", got)
	}
	if r.count != 5 {
		t.Fatalf("count = %d, want 5", r.count)
	}
}

func TestCountingLimitReaderExactlyAtLimitSucceeds(t *testing.T) {
	src := bytes.NewReader([]byte("exactly10!"))
	r := &countingLimitReader{r: src, remaining: 10}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll error = %v, want nil at exact limit", err)
	}
	if string(got) != "exactly10!" || r.count != 10 {
		t.Fatalf("read %q count %d, want exactly10! / 10", got, r.count)
	}
}

func TestCountingLimitReaderOverLimitAbortsAtBoundary(t *testing.T) {
	src := strings.NewReader("this payload is well over the limit")
	r := &countingLimitReader{r: src, remaining: 8}
	got, err := io.ReadAll(r)
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("ReadAll error = %v, want errTooLarge", err)
	}
	if r.count != 8 {
		t.Fatalf("count = %d, want 8 (must stop exactly at the cap)", r.count)
	}
	if len(got) != 8 {
		t.Fatalf("read %d bytes, want 8", len(got))
	}
}
