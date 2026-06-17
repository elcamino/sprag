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

import { lazy, Suspense } from "react";
import { BrandMotif } from "./BrandMotif";
import { ThemeSwitch } from "./ThemeSwitch";
import { routeForPath } from "./routes";

const Home = lazy(() => import("./pages/Home"));
const Upload = lazy(() => import("./pages/Upload"));
const AdminLogin = lazy(() => import("./pages/AdminLogin"));
const AdminDashboard = lazy(() => import("./pages/AdminDashboard"));

export default function App() {
  const routeKind = routeForPath(window.location.pathname);
  const route =
    routeKind === "home" ? (
      <Home />
    ) : routeKind === "admin-login" ? (
      <AdminLogin />
    ) : routeKind === "admin-dashboard" ? (
      <AdminDashboard />
    ) : (
      <Upload />
    );

  return (
    <>
      {routeKind !== "home" && <BrandMotif />}
      {routeKind !== "admin-dashboard" && <ThemeSwitch fixed />}
      <Suspense fallback={<div className="route-loading">Zener</div>}>{route}</Suspense>
    </>
  );
}
