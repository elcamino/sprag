/*
 * Zener - a post-quantum-safe end-to-end encrypted file dropbox.
 * Copyright (C) 2026 Tobias von Dewitz <tobias@vondewitz.org>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

import { describe, expect, it } from "vitest";
import {
  E2E_ALGORITHM,
  decryptEncryptedUpload,
  encryptFileForPage,
  exportPrivateIdentity,
  generateE2EIdentity,
  parsePrivateIdentity,
  publicIdentityFromPrivate
} from "./e2eCrypto";

describe("E2E crypto", () => {
  it("generates an exportable identity and derives the same public identity after import", async () => {
    const identity = await generateE2EIdentity();
    const exported = exportPrivateIdentity(identity);
    const imported = parsePrivateIdentity(exported);
    const publicIdentity = publicIdentityFromPrivate(imported);

    expect(imported.algorithm).toBe(E2E_ALGORITHM);
    expect(publicIdentity.publicKey).toBe(identity.publicIdentity.publicKey);
    expect(publicIdentity.fingerprint).toBe(identity.publicIdentity.fingerprint);
    expect(exported).toContain("secretKey");
  });

  it("encrypts file bytes and metadata for a public identity and decrypts with the private identity", async () => {
    const identity = await generateE2EIdentity();
    const file = new File(["privileged contents"], "privileged-report.pdf", {
      type: "application/pdf",
      lastModified: Date.UTC(2026, 5, 17, 12, 0, 0)
    });

    const encrypted = await encryptFileForPage(file, {
      enabled: true,
      algorithm: E2E_ALGORITHM,
      public_key: JSON.stringify(identity.publicIdentity),
      public_key_fingerprint: identity.publicIdentity.fingerprint
    });

    expect(encrypted.uploadFile.name).not.toContain("privileged-report.pdf");
    expect(encrypted.uploadFile.type).toBe("application/octet-stream");
    expect(encrypted.envelope).not.toContain("privileged-report.pdf");
    expect(encrypted.envelope).toContain(identity.publicIdentity.fingerprint);

    const decrypted = await decryptEncryptedUpload(
      await encrypted.uploadFile.arrayBuffer(),
      encrypted.envelope,
      identity.privateIdentity
    );

    expect(decrypted.name).toBe("privileged-report.pdf");
    expect(decrypted.type).toBe("application/pdf");
    expect(decrypted.size).toBe(file.size);
    expect(await decrypted.blob.text()).toBe("privileged contents");
  });
});
