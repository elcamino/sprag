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

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "—";
  }
  const total = Math.round(seconds);
  if (total < 60) {
    return `${total}s`;
  }
  if (total < 3600) {
    const minutes = Math.floor(total / 60);
    const rest = total % 60;
    return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;
  }
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`;
}

export type TransferSample = {
  bytesPerSecond: number;
  etaSeconds: number | null;
};

export type TransferEstimator = {
  update(loaded: number, total: number, nowMs: number): TransferSample;
};

export function createTransferEstimator(): TransferEstimator {
  let startMs: number | null = null;
  let startLoaded = 0;

  return {
    update(loaded: number, total: number, nowMs: number): TransferSample {
      // Anchor on the first sample, then measure throughput as cumulative bytes
      // over cumulative wall-clock time. XHR upload progress reports bytes
      // written to the socket send buffer, so it arrives in fast bursts
      // separated by slow network drains; an instantaneous per-sample rate is
      // biased toward the burst speed and badly overestimates throughput.
      // Averaging from a fixed anchor divides by real elapsed time and is immune
      // to that sampling bias.
      if (startMs === null) {
        startMs = nowMs;
        startLoaded = loaded;
        return { bytesPerSecond: 0, etaSeconds: loaded >= total ? 0 : null };
      }
      const elapsedMs = nowMs - startMs;
      const bytes = loaded - startLoaded;
      const bytesPerSecond = elapsedMs > 0 && bytes > 0 ? (bytes / elapsedMs) * 1000 : 0;
      if (loaded >= total) {
        return { bytesPerSecond, etaSeconds: 0 };
      }
      const etaSeconds = bytesPerSecond > 0 ? (total - loaded) / bytesPerSecond : null;
      return { bytesPerSecond, etaSeconds };
    }
  };
}
