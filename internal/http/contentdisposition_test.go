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

package httpapi

import (
	"strings"
	"testing"
)

func TestContentDispositionEncodesNonASCIIFilename(t *testing.T) {
	got := contentDisposition("Lebenslauf Müller.pdf")
	if !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("not an attachment disposition: %q", got)
	}
	if !strings.Contains(got, `filename="`) {
		t.Fatalf("missing quoted ASCII fallback: %q", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''") {
		t.Fatalf("missing RFC 5987 filename*: %q", got)
	}
	// "ü" is U+00FC -> UTF-8 0xC3 0xBC, and a space -> %20.
	if !strings.Contains(got, "Lebenslauf%20M%C3%BCller.pdf") {
		t.Fatalf("filename* not percent-encoded as UTF-8: %q", got)
	}
}

func TestContentDispositionPlainASCII(t *testing.T) {
	got := contentDisposition("report.pdf")
	if !strings.Contains(got, `filename="report.pdf"`) {
		t.Fatalf("expected plain ASCII filename, got %q", got)
	}
}

func TestContentDispositionQuotesAreNeutralizedInFallback(t *testing.T) {
	got := contentDisposition(`a"b\c.txt`)
	if strings.Contains(got, `filename="a"b`) {
		t.Fatalf("unescaped quote leaked into fallback: %q", got)
	}
}
