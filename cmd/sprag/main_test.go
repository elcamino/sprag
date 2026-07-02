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

// The listener needs header and idle timeouts so idle or half-open connections
// cannot pile up, but must NOT set ReadTimeout or WriteTimeout: multi-gigabyte
// uploads and downloads over slow links are legitimate and would be killed
// mid-transfer by any whole-request deadline.
func TestNewHTTPServerBoundsConnectionLifetimes(t *testing.T) {
	server := newHTTPServer("8080", nil)

	if server.Addr != ":8080" {
		t.Fatalf("Addr = %q, want :8080", server.Addr)
	}
	if server.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be set to bound slow-header connections")
	}
	if server.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout must be set to reap idle keep-alive connections")
	}
	if server.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %v, want 0 (would kill slow large uploads)", server.ReadTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v, want 0 (would kill slow large downloads)", server.WriteTimeout)
	}
}

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
