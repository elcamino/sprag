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

package e2e

const (
	Algorithm                   = "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM"
	UploadMode                  = "e2e-v1"
	CiphertextOverheadAllowance = 1024
)

func SupportedAlgorithm(algorithm string) bool {
	return algorithm == Algorithm
}
