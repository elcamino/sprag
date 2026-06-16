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

import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "var(--text-strong)",
        paper: "var(--bg-canvas)",
        graphite: "var(--text)",
        moss: "var(--success)",
        reed: "var(--accent-soft)",
        signal: "var(--accent)",
        oxide: "var(--danger)"
      },
      fontFamily: {
        sans: ['"Hanken Grotesk Variable"', "system-ui", "sans-serif"]
      },
      boxShadow: {
        line: "0 1px 0 var(--border)",
        lift: "var(--shadow-card)"
      }
    }
  },
  plugins: []
} satisfies Config;
