// Zener - a post-quantum-safe end-to-end encrypted file dropbox.
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
import { generateCircuitPaths, influencePoint } from "./circuitLineField";

describe("generateCircuitPaths", () => {
  it("generates deterministic paths for the same seed and dimensions", () => {
    const first = generateCircuitPaths({ width: 800, height: 600, seed: 42 });
    const second = generateCircuitPaths({ width: 800, height: 600, seed: 42 });

    expect(first).toEqual(second);
    expect(first.length).toBeGreaterThan(12);
  });

  it("keeps every generated point inside the canvas bounds", () => {
    const paths = generateCircuitPaths({ width: 360, height: 240, seed: 7 });

    for (const path of paths) {
      for (const point of path.points) {
        expect(point.x).toBeGreaterThanOrEqual(0);
        expect(point.x).toBeLessThanOrEqual(360);
        expect(point.y).toBeGreaterThanOrEqual(0);
        expect(point.y).toBeLessThanOrEqual(240);
      }
    }
  });
});

describe("influencePoint", () => {
  it("moves nearby points and leaves distant points unchanged", () => {
    const near = influencePoint({ x: 100, y: 100 }, { x: 120, y: 100, active: true }, 160, 24);
    const far = influencePoint({ x: 100, y: 100 }, { x: 420, y: 100, active: true }, 160, 24);

    expect(near.x).toBeGreaterThan(100);
    expect(near.y).toBe(100);
    expect(far).toEqual({ x: 100, y: 100 });
  });
});
