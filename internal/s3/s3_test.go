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

package s3store

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestUploadInputRequestsStoredChecksum(t *testing.T) {
	input := uploadInput("bucket", "object-key", strings.NewReader("payload"), "text/plain")

	if input.ChecksumAlgorithm != types.ChecksumAlgorithmCrc32 {
		t.Fatalf("expected CRC32 checksum algorithm, got %q", input.ChecksumAlgorithm)
	}
	if input.ContentType == nil || *input.ContentType != "text/plain" {
		t.Fatalf("expected content type to be preserved, got %v", input.ContentType)
	}
}

func TestDownloadInputRequestsChecksumValidation(t *testing.T) {
	input := downloadInput("bucket", "object-key")

	if input.ChecksumMode != types.ChecksumModeEnabled {
		t.Fatalf("expected checksum validation mode enabled, got %q", input.ChecksumMode)
	}
}
