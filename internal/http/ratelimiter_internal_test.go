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
	"fmt"
	"testing"
	"time"
)

func TestRateLimiterEvictsExpiredBuckets(t *testing.T) {
	r := newRateLimiter()
	base := time.Unix(1_700_000_000, 0)

	// Populate many distinct keys within a single window.
	for i := 0; i < 1000; i++ {
		r.Allow(fmt.Sprintf("ip-%d", i), 5, time.Minute, base)
	}
	if len(r.buckets) != 1000 {
		t.Fatalf("expected 1000 buckets while active, got %d", len(r.buckets))
	}

	// A request after the window must evict the stale keys, not accumulate them.
	later := base.Add(2 * time.Minute)
	r.Allow("fresh", 5, time.Minute, later)
	if len(r.buckets) != 1 {
		t.Fatalf("expected stale buckets evicted leaving 1, got %d", len(r.buckets))
	}
}

func TestRateLimiterKeepsActiveBucketsDuringSweep(t *testing.T) {
	r := newRateLimiter()
	base := time.Unix(1_700_000_000, 0)

	r.Allow("stale", 5, time.Minute, base)
	r.Allow("active", 5, time.Minute, base.Add(50*time.Second))

	// At base+70s the stale bucket (70s old) is past the window and evicted,
	// while the active bucket (20s old) survives and keeps counting.
	sweep := base.Add(70 * time.Second)
	r.Allow("active", 5, time.Minute, sweep)

	if _, ok := r.buckets["stale"]; ok {
		t.Fatal("stale bucket should have been evicted")
	}
	bucket, ok := r.buckets["active"]
	if !ok {
		t.Fatal("active bucket should survive the sweep")
	}
	if bucket.count != 2 {
		t.Fatalf("active bucket count = %d, want 2", bucket.count)
	}
}
