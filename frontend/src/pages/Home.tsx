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

import { ArrowRight } from "lucide-react";
import { RootLineField } from "../RootLineField";

export default function Home() {
  return (
    <main className="home-shell">
      <RootLineField />
      <a className="home-admin-link" href="/admin" aria-label="Open admin">
        <span>Admin</span>
        <ArrowRight size={16} />
      </a>
      <h1 className="home-mark" aria-label="Zener">
        Z
      </h1>
    </main>
  );
}
