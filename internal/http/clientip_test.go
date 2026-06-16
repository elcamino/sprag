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
	"net/http"
	"testing"
)

func requestWith(remoteAddr, xff string) *http.Request {
	r := &http.Request{Header: http.Header{}, RemoteAddr: remoteAddr}
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestClientIPIgnoresForwardedHeaderWithZeroTrustedHops(t *testing.T) {
	// Directly exposed: X-Forwarded-For is fully attacker-controlled and must be ignored.
	r := requestWith("198.51.100.9:54321", "1.2.3.4, 5.6.7.8")
	if got := clientIP(r, 0); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want the direct peer 198.51.100.9", got)
	}
}

func TestClientIPTrustsProxyAppendedEntryWithOneHop(t *testing.T) {
	// Behind a single proxy (Caddy), the rightmost entry is the one the proxy appended.
	r := requestWith("10.0.0.2:1234", "203.0.113.5, 198.51.100.9")
	if got := clientIP(r, 1); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want proxy-appended 198.51.100.9", got)
	}
}

func TestClientIPRejectsSpoofedLeftmostEntry(t *testing.T) {
	// Attacker prepends a fake IP; the proxy still appends the real peer on the right.
	r := requestWith("10.0.0.2:1234", "evil-spoof, 198.51.100.9")
	if got := clientIP(r, 1); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want real peer 198.51.100.9 (spoof must not win)", got)
	}
}

func TestClientIPHandlesSingleForwardedEntry(t *testing.T) {
	r := requestWith("10.0.0.2:1234", "198.51.100.9")
	if got := clientIP(r, 1); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want 198.51.100.9", got)
	}
}

func TestClientIPCountsMultipleTrustedHops(t *testing.T) {
	// Two trusted proxies append two entries; the client sits at len-2.
	r := requestWith("10.0.0.2:1234", "evil-spoof, 203.0.113.5, 198.51.100.9")
	if got := clientIP(r, 2); got != "203.0.113.5" {
		t.Fatalf("clientIP = %q, want client at len-hops 203.0.113.5", got)
	}
}

func TestClientIPFallsBackToPeerWhenForwardedMissing(t *testing.T) {
	r := requestWith("198.51.100.9:54321", "")
	if got := clientIP(r, 1); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want peer fallback 198.51.100.9", got)
	}
}

func TestClientIPClampsHopsLargerThanChain(t *testing.T) {
	// If configured hops exceeds the actual chain, use the leftmost entry rather than panicking.
	r := requestWith("10.0.0.2:1234", "198.51.100.9")
	if got := clientIP(r, 5); got != "198.51.100.9" {
		t.Fatalf("clientIP = %q, want clamped leftmost 198.51.100.9", got)
	}
}
