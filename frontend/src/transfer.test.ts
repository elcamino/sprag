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

import { describe, expect, it } from "vitest";
import { formatDuration } from "./transfer";

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
