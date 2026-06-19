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

package main

import (
	"testing"

	"github.com/elcamino/sprag/internal/config"
)

func TestNewHTTPConfigPreservesSecureCookieSetting(t *testing.T) {
	cfg := config.Config{
		BaseURL:           "http://abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcd.onion",
		SessionSecret:     []byte("12345678901234567890123456789012"),
		AdminUsername:     "admin",
		AdminPasswordHash: "$2a$10$u2jdWdEhSWnztZ0ynTflA.X2tqztNA25sWwliWeqTCvS5Dj5slUaC",
		IPStorageMode:     "hmac-sha256",
		IPHashSecret:      []byte("12345678901234567890123456789012"),
		MaxFileSize:       1024,
		AllowedExtensions: []string{"pdf"},
		TrustedProxyHops:  0,
		SecureCookies:     false,
		AnonymousIngress:  true,
		E2EIntake: config.E2EConfig{
			Enabled:   true,
			Required:  true,
			Algorithm: "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM",
		},
		S3: config.S3Config{Prefix: "incoming/"},
	}

	httpCfg := newHTTPConfig(cfg)

	if httpCfg.SecureCookies {
		t.Fatal("expected HTTP config to preserve SecureCookies=false")
	}
	if httpCfg.BaseURL != cfg.BaseURL {
		t.Fatalf("BaseURL = %q, want %q", httpCfg.BaseURL, cfg.BaseURL)
	}
	if httpCfg.TrustedProxyHops != 0 {
		t.Fatalf("TrustedProxyHops = %d, want 0", httpCfg.TrustedProxyHops)
	}
	if !httpCfg.AnonymousIngress {
		t.Fatal("expected HTTP config to preserve AnonymousIngress=true")
	}
	if !httpCfg.E2EIntake.Enabled || !httpCfg.E2EIntake.Required {
		t.Fatalf("E2EIntake = %#v, want enabled and required", httpCfg.E2EIntake)
	}
}
