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
        ink: "#11130f",
        paper: "#f7f4ed",
        graphite: "#343a32",
        moss: "#2f6b4f",
        reed: "#c4d8a8",
        signal: "#d9a441",
        oxide: "#a44a3f"
      },
      boxShadow: {
        line: "0 1px 0 rgba(17,19,15,0.08)",
        lift: "0 16px 40px rgba(17,19,15,0.12)"
      }
    }
  },
  plugins: []
} satisfies Config;
