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

export type ThemeMode = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";

export const STORAGE_KEY = "zener-theme";

export function readMode(storage: Pick<Storage, "getItem">): ThemeMode {
  const value = storage.getItem(STORAGE_KEY);
  return value === "light" || value === "dark" || value === "system" ? value : "system";
}

export function writeMode(storage: Pick<Storage, "setItem" | "removeItem">, mode: ThemeMode): void {
  if (mode === "system") {
    storage.removeItem(STORAGE_KEY);
  } else {
    storage.setItem(STORAGE_KEY, mode);
  }
}

export function resolveMode(mode: ThemeMode, prefersDark: boolean): ResolvedTheme {
  if (mode === "system") {
    return prefersDark ? "dark" : "light";
  }
  return mode;
}

export function applyMode(root: Pick<Element, "setAttribute" | "removeAttribute">, mode: ThemeMode): void {
  if (mode === "system") {
    root.removeAttribute("data-theme");
  } else {
    root.setAttribute("data-theme", mode);
  }
}

export type ThemeController = {
  getMode: () => ThemeMode;
  setMode: (mode: ThemeMode) => void;
  subscribe: (listener: () => void) => () => void;
};

export function createThemeController(win: Window = window): ThemeController {
  const storage = win.localStorage;
  const root = win.document.documentElement;
  const media = win.matchMedia("(prefers-color-scheme: dark)");
  const listeners = new Set<() => void>();
  let mode = readMode(storage);

  applyMode(root, mode);
  const notify = () => listeners.forEach((listener) => listener());
  media.addEventListener("change", () => {
    if (mode === "system") notify();
  });

  return {
    getMode: () => mode,
    setMode: (next) => {
      mode = next;
      writeMode(storage, next);
      applyMode(root, next);
      notify();
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    }
  };
}
