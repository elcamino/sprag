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

import { describe, expect, it } from "vitest";
import { createTransferEstimator, formatDuration } from "./transfer";

describe("formatDuration", () => {
  it("formats whole seconds", () => {
    expect(formatDuration(8)).toBe("8s");
  });

  it("rounds to whole seconds", () => {
    expect(formatDuration(8.4)).toBe("8s");
    expect(formatDuration(8.6)).toBe("9s");
  });

  it("formats minutes and seconds", () => {
    expect(formatDuration(80)).toBe("1m 20s");
  });

  it("drops the seconds when they are zero", () => {
    expect(formatDuration(120)).toBe("2m");
  });

  it("formats hours and minutes", () => {
    expect(formatDuration(7500)).toBe("2h 5m");
    expect(formatDuration(7200)).toBe("2h");
  });

  it("returns a dash for unknown durations", () => {
    expect(formatDuration(Number.NaN)).toBe("—");
    expect(formatDuration(Number.POSITIVE_INFINITY)).toBe("—");
    expect(formatDuration(-5)).toBe("—");
  });
});

const MIB = 1024 * 1024;

describe("createTransferEstimator", () => {
  it("returns a null eta on the first sample", () => {
    const estimator = createTransferEstimator();
    const result = estimator.update(0, 10 * MIB, 0);
    expect(result.etaSeconds).toBeNull();
  });

  it("estimates rate and eta from a steady stream", () => {
    const estimator = createTransferEstimator();
    estimator.update(0, 10 * MIB, 0);
    // 1 MiB transferred in 100 ms -> 10 MiB/s.
    const result = estimator.update(1 * MIB, 10 * MIB, 100);
    expect(result.bytesPerSecond).toBeGreaterThan(10 * MIB * 0.99);
    expect(result.bytesPerSecond).toBeLessThan(10 * MIB * 1.01);
    // 9 MiB remaining at 10 MiB/s -> ~0.9 s.
    expect(result.etaSeconds).not.toBeNull();
    expect(result.etaSeconds as number).toBeGreaterThan(0.85);
    expect(result.etaSeconds as number).toBeLessThan(0.95);
  });

  it("stays realistic when upload progress arrives in fast bursts", () => {
    // XHR upload progress reports bytes written to the socket buffer, not bytes
    // on the wire. A slow ~1 MiB/s upload therefore arrives as fast bursts: each
    // ~1 MiB is written to the socket in ~10 ms, once per second, with the
    // network draining the buffer in between. An instantaneous-rate model is
    // biased toward the burst speed and grossly overestimates throughput.
    const estimator = createTransferEstimator();
    const total = 100 * MIB;
    estimator.update(0, total, 0);
    let last = estimator.update(0, total, 0);
    for (let sec = 1; sec <= 60; sec++) {
      // mid-burst sample, then the burst completes 10 ms later
      estimator.update((sec - 0.5) * MIB, total, sec * 1000 + 10);
      last = estimator.update(sec * MIB, total, sec * 1000 + 20);
    }
    // 60 MiB sent in ~60 s -> sustained ~1 MiB/s -> ~40 s remaining.
    expect(last.etaSeconds).not.toBeNull();
    expect(last.etaSeconds as number).toBeGreaterThan(25);
    expect(last.etaSeconds as number).toBeLessThan(60);
  });

  it("reports the overall average, not the latest interval's instant rate", () => {
    const estimator = createTransferEstimator();
    const total = 100 * MIB;
    estimator.update(0, total, 0);
    estimator.update(50 * MIB, total, 1000); // 50 MiB in 1 s
    // 10 s later only 1 more MiB arrived (a near-stall). An instantaneous model
    // would read ~0.1 MiB/s here; the overall average is 51 MiB / 11 s.
    const result = estimator.update(51 * MIB, total, 11000);
    expect(result.bytesPerSecond).toBeGreaterThan(4 * MIB);
    expect(result.bytesPerSecond).toBeLessThan(5.5 * MIB);
    // 49 MiB remaining at ~4.6 MiB/s -> ~10-11 s.
    expect(result.etaSeconds as number).toBeGreaterThan(8);
    expect(result.etaSeconds as number).toBeLessThan(13);
  });

  it("returns null until real time and bytes have elapsed", () => {
    const estimator = createTransferEstimator();
    estimator.update(0, 10 * MIB, 0); // anchor
    // Same timestamp, no progress: cannot compute a rate yet.
    const stalled = estimator.update(0, 10 * MIB, 0);
    expect(stalled.bytesPerSecond).toBe(0);
    expect(stalled.etaSeconds).toBeNull();
    // A later repeated timestamp must never produce NaN/Infinity.
    estimator.update(1 * MIB, 10 * MIB, 100);
    const repeated = estimator.update(2 * MIB, 10 * MIB, 100);
    expect(Number.isFinite(repeated.bytesPerSecond)).toBe(true);
    expect(repeated.etaSeconds === null || Number.isFinite(repeated.etaSeconds)).toBe(true);
  });

  it("reports a zero eta at completion, even as the first sample", () => {
    const estimator = createTransferEstimator();
    const result = estimator.update(10 * MIB, 10 * MIB, 50);
    expect(result.etaSeconds).toBe(0);
  });
});
