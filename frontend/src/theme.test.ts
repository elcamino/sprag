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
import { applyMode, readMode, resolveMode, STORAGE_KEY, writeMode } from "./theme";

function fakeStorage(initial: Record<string, string> = {}) {
  const map = new Map(Object.entries(initial));
  return {
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
    has: (k: string) => map.has(k),
    value: (k: string) => map.get(k)
  };
}

function fakeRoot() {
  const attrs: Record<string, string> = {};
  return {
    setAttribute: (k: string, v: string) => void (attrs[k] = v),
    removeAttribute: (k: string) => void delete attrs[k],
    get: (k: string) => attrs[k]
  };
}

describe("readMode", () => {
  it("uses a Sprag-specific storage key", () => {
    expect(STORAGE_KEY).toBe("sprag-theme");
  });

  it("defaults to system when empty", () => {
    expect(readMode(fakeStorage())).toBe("system");
  });
  it("defaults to system for unknown values", () => {
    expect(readMode(fakeStorage({ [STORAGE_KEY]: "weird" }))).toBe("system");
  });
  it("returns the stored mode", () => {
    expect(readMode(fakeStorage({ [STORAGE_KEY]: "light" }))).toBe("light");
    expect(readMode(fakeStorage({ [STORAGE_KEY]: "dark" }))).toBe("dark");
    expect(readMode(fakeStorage({ [STORAGE_KEY]: "system" }))).toBe("system");
  });
});

describe("writeMode", () => {
  it("persists light and dark", () => {
    const s = fakeStorage();
    writeMode(s, "dark");
    expect(s.value(STORAGE_KEY)).toBe("dark");
  });
  it("clears the override for system", () => {
    const s = fakeStorage({ [STORAGE_KEY]: "dark" });
    writeMode(s, "system");
    expect(s.has(STORAGE_KEY)).toBe(false);
  });
});

describe("resolveMode", () => {
  it("follows the OS in system mode", () => {
    expect(resolveMode("system", true)).toBe("dark");
    expect(resolveMode("system", false)).toBe("light");
  });
  it("honours explicit modes regardless of OS", () => {
    expect(resolveMode("light", true)).toBe("light");
    expect(resolveMode("dark", false)).toBe("dark");
  });
});

describe("applyMode", () => {
  it("sets data-theme for explicit modes", () => {
    const r = fakeRoot();
    applyMode(r, "dark");
    expect(r.get("data-theme")).toBe("dark");
    applyMode(r, "light");
    expect(r.get("data-theme")).toBe("light");
  });
  it("removes data-theme for system", () => {
    const r = fakeRoot();
    applyMode(r, "dark");
    applyMode(r, "system");
    expect(r.get("data-theme")).toBeUndefined();
  });
});
