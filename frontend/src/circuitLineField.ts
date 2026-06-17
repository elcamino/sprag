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

export type CircuitPoint = {
  x: number;
  y: number;
};

export type CircuitPath = {
  points: CircuitPoint[];
  accent: boolean;
};

export type PointerState = {
  x: number;
  y: number;
  active: boolean;
};

export type GenerateCircuitOptions = {
  width: number;
  height: number;
  seed: number;
};

type Random = () => number;

export function generateCircuitPaths({ width, height, seed }: GenerateCircuitOptions): CircuitPath[] {
  const random = createRandom(seed);
  const count = Math.max(14, Math.round((width * height) / 42000));
  const paths: CircuitPath[] = [];
  const safeWidth = Math.max(1, width);
  const safeHeight = Math.max(1, height);
  const center = { x: safeWidth / 2, y: safeHeight / 2 };
  const quietRadius = Math.min(safeWidth, safeHeight) * 0.18;

  for (let index = 0; index < count; index += 1) {
    const horizontal = random() > 0.34;
    const start = pickPoint(random, safeWidth, safeHeight, center, quietRadius);
    const segmentCount = 3 + Math.floor(random() * 4);
    const points: CircuitPoint[] = [start];
    let current = start;

    for (let segment = 0; segment < segmentCount; segment += 1) {
      const length = 56 + random() * 150;
      const direction = random() > 0.5 ? 1 : -1;
      const bend = random() > 0.58 ? 0.5 : 0;
      const dx = horizontal === (segment % 2 === 0) ? length * direction : length * bend * direction;
      const dy = horizontal === (segment % 2 === 0) ? length * bend * direction : length * direction;
      current = clampPoint({ x: current.x + dx, y: current.y + dy }, safeWidth, safeHeight);
      points.push(current);
    }

    if (points.length > 2) {
      paths.push({ points, accent: index % 5 === 0 });
    }
  }

  return paths;
}

export function influencePoint(
  point: CircuitPoint,
  pointer: PointerState,
  radius: number,
  strength: number
): CircuitPoint {
  if (!pointer.active) {
    return point;
  }
  const dx = pointer.x - point.x;
  const dy = pointer.y - point.y;
  const distance = Math.hypot(dx, dy);
  if (distance === 0 || distance > radius) {
    return point;
  }
  const force = (1 - distance / radius) * strength;
  return {
    x: point.x + (dx / distance) * force,
    y: point.y + (dy / distance) * force
  };
}

function pickPoint(random: Random, width: number, height: number, center: CircuitPoint, quietRadius: number): CircuitPoint {
  for (let tries = 0; tries < 8; tries += 1) {
    const point = { x: random() * width, y: random() * height };
    if (Math.hypot(point.x - center.x, point.y - center.y) > quietRadius) {
      return point;
    }
  }
  return { x: random() * width, y: random() * height };
}

function clampPoint(point: CircuitPoint, width: number, height: number): CircuitPoint {
  return {
    x: Math.min(width, Math.max(0, point.x)),
    y: Math.min(height, Math.max(0, point.y))
  };
}

function createRandom(seed: number): Random {
  let state = seed >>> 0;
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0;
    return state / 4294967296;
  };
}
