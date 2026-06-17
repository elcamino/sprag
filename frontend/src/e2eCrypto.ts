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

import { ml_kem1024 } from "@noble/post-quantum/ml-kem.js";
import { randomBytes } from "@noble/post-quantum/utils.js";

export const E2E_ALGORITHM = "ML-KEM-1024-P384-HKDF-SHA512-AES-256-GCM";

const KEY_TYPE_PUBLIC = "e2e-public-key";
const KEY_TYPE_PRIVATE = "e2e-private-key";
const VERSION = 1;
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export type E2EPublicIdentity = {
  sprag: typeof KEY_TYPE_PUBLIC;
  version: typeof VERSION;
  algorithm: typeof E2E_ALGORITHM;
  publicKey: string;
  ecdhPublicKey: JsonWebKey;
  fingerprint: string;
};

export type E2EPrivateIdentity = {
  sprag: typeof KEY_TYPE_PRIVATE;
  version: typeof VERSION;
  algorithm: typeof E2E_ALGORITHM;
  publicIdentity: E2EPublicIdentity;
  secretKey: string;
  ecdhPrivateKey: JsonWebKey;
};

export type GeneratedE2EIdentity = {
  publicIdentity: E2EPublicIdentity;
  privateIdentity: E2EPrivateIdentity;
};

export type E2EPageConfig = {
  enabled: true;
  algorithm: string;
  public_key: string;
  public_key_fingerprint: string;
};

export type EncryptedUpload = {
  uploadFile: File;
  envelope: string;
};

export type DecryptedUpload = {
  name: string;
  type: string;
  size: number;
  blob: Blob;
};

type FileMetadata = {
  name: string;
  size: number;
  content_type: string;
  last_modified: number;
};

type E2EEnvelope = {
  version: typeof VERSION;
  algorithm: typeof E2E_ALGORITHM;
  public_key_fingerprint: string;
  kem_ciphertext: string;
  ecdh_ephemeral_public_key: string;
  salt: string;
  file_nonce: string;
  metadata_nonce: string;
  encrypted_metadata: string;
};

export async function generateE2EIdentity(): Promise<GeneratedE2EIdentity> {
  const kem = ml_kem1024.keygen();
  const ecdh = await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-384" }, true, ["deriveBits"]);
  const ecdhPublicKey = await crypto.subtle.exportKey("jwk", ecdh.publicKey);
  const ecdhPrivateKey = await crypto.subtle.exportKey("jwk", ecdh.privateKey);
  const publicBody = {
    sprag: KEY_TYPE_PUBLIC,
    version: VERSION,
    algorithm: E2E_ALGORITHM,
    publicKey: bytesToBase64URL(kem.publicKey),
    ecdhPublicKey
  } satisfies Omit<E2EPublicIdentity, "fingerprint">;
  const fingerprint = await publicKeyFingerprint(publicBody);
  const publicIdentity: E2EPublicIdentity = { ...publicBody, fingerprint };
  const privateIdentity: E2EPrivateIdentity = {
    sprag: KEY_TYPE_PRIVATE,
    version: VERSION,
    algorithm: E2E_ALGORITHM,
    publicIdentity,
    secretKey: bytesToBase64URL(kem.secretKey),
    ecdhPrivateKey
  };
  return { publicIdentity, privateIdentity };
}

export function exportPrivateIdentity(identity: GeneratedE2EIdentity | E2EPrivateIdentity): string {
  const privateIdentity = "privateIdentity" in identity ? identity.privateIdentity : identity;
  return JSON.stringify(privateIdentity, null, 2);
}

export function parsePrivateIdentity(raw: string): E2EPrivateIdentity {
  let parsed: E2EPrivateIdentity;
  try {
    parsed = JSON.parse(raw) as E2EPrivateIdentity;
  } catch {
    throw new Error("Private key must be valid JSON");
  }
  if (parsed.sprag !== KEY_TYPE_PRIVATE || parsed.version !== VERSION || parsed.algorithm !== E2E_ALGORITHM) {
    throw new Error("Private key is not a supported Sprag E2E key");
  }
  if (!parsed.secretKey || !parsed.ecdhPrivateKey || !parsed.publicIdentity) {
    throw new Error("Private key is missing key material");
  }
  return parsed;
}

export function publicIdentityFromPrivate(identity: E2EPrivateIdentity): E2EPublicIdentity {
  return identity.publicIdentity;
}

export function parsePublicIdentity(raw: string): E2EPublicIdentity {
  let parsed: E2EPublicIdentity;
  try {
    parsed = JSON.parse(raw) as E2EPublicIdentity;
  } catch {
    throw new Error("Public key must be valid JSON");
  }
  if (parsed.sprag !== KEY_TYPE_PUBLIC || parsed.version !== VERSION || parsed.algorithm !== E2E_ALGORITHM) {
    throw new Error("Public key is not a supported Sprag E2E key");
  }
  if (!parsed.publicKey || !parsed.ecdhPublicKey || !parsed.fingerprint) {
    throw new Error("Public key is missing key material");
  }
  return parsed;
}

export async function encryptFileForPage(file: File, page: E2EPageConfig): Promise<EncryptedUpload> {
  if (page.algorithm !== E2E_ALGORITHM) {
    throw new Error("Unsupported E2E algorithm");
  }
  const publicIdentity = parsePublicIdentity(page.public_key);
  if (publicIdentity.fingerprint !== page.public_key_fingerprint) {
    throw new Error("E2E public key fingerprint mismatch");
  }
  const verifiedFingerprint = await publicKeyFingerprint({
    sprag: publicIdentity.sprag,
    version: publicIdentity.version,
    algorithm: publicIdentity.algorithm,
    publicKey: publicIdentity.publicKey,
    ecdhPublicKey: publicIdentity.ecdhPublicKey
  });
  if (verifiedFingerprint !== publicIdentity.fingerprint) {
    throw new Error("E2E public key fingerprint is invalid");
  }

  const kem = ml_kem1024.encapsulate(base64URLToBytes(publicIdentity.publicKey));
  const recipientECDH = await crypto.subtle.importKey(
    "jwk",
    publicIdentity.ecdhPublicKey,
    { name: "ECDH", namedCurve: "P-384" },
    false,
    []
  );
  const ephemeral = await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-384" }, true, ["deriveBits"]);
  const ecdhSecret = await crypto.subtle.deriveBits({ name: "ECDH", public: recipientECDH }, ephemeral.privateKey, 384);
  const ephemeralPublicKey = await crypto.subtle.exportKey("jwk", ephemeral.publicKey);
  const salt = randomBytes(32);
  const fileNonce = randomBytes(12);
  const metadataNonce = randomBytes(12);
  const sharedMaterial = concatBytes(kem.sharedSecret, new Uint8Array(ecdhSecret));
  const coreEnvelope = {
    version: VERSION as typeof VERSION,
    algorithm: E2E_ALGORITHM as typeof E2E_ALGORITHM,
    public_key_fingerprint: publicIdentity.fingerprint,
    kem_ciphertext: bytesToBase64URL(kem.cipherText),
    ecdh_ephemeral_public_key: stringToBase64URL(canonicalJSONString(ephemeralPublicKey)),
    salt: bytesToBase64URL(salt),
    file_nonce: bytesToBase64URL(fileNonce),
    metadata_nonce: bytesToBase64URL(metadataNonce)
  };
  const metadataKey = await deriveAESKey(sharedMaterial, salt, await kdfInfo("metadata-key", coreEnvelope), ["encrypt"]);
  const fileKey = await deriveAESKey(sharedMaterial, salt, await kdfInfo("file-key", coreEnvelope), ["encrypt"]);
  const metadata: FileMetadata = {
    name: file.name,
    size: file.size,
    content_type: file.type,
    last_modified: file.lastModified
  };
  const encryptedMetadata = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: bufferSource(metadataNonce), additionalData: aad("metadata", coreEnvelope), tagLength: 128 },
    metadataKey,
    textEncoder.encode(JSON.stringify(metadata))
  );
  const ciphertext = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: bufferSource(fileNonce), additionalData: aad("file", coreEnvelope), tagLength: 128 },
    fileKey,
    await file.arrayBuffer()
  );
  const envelope: E2EEnvelope = {
    ...coreEnvelope,
    encrypted_metadata: bytesToBase64URL(new Uint8Array(encryptedMetadata))
  };
  return {
    uploadFile: new File([new Uint8Array(ciphertext)], `${crypto.randomUUID()}.sprag`, { type: "application/octet-stream" }),
    envelope: JSON.stringify(envelope)
  };
}

export async function decryptEncryptedUpload(
  ciphertext: ArrayBuffer,
  rawEnvelope: string,
  privateIdentity: E2EPrivateIdentity
): Promise<DecryptedUpload> {
  const envelope = parseEnvelope(rawEnvelope);
  if (envelope.public_key_fingerprint !== privateIdentity.publicIdentity.fingerprint) {
    throw new Error("Private key does not match this encrypted upload");
  }
  const kemSecret = ml_kem1024.decapsulate(base64URLToBytes(envelope.kem_ciphertext), base64URLToBytes(privateIdentity.secretKey));
  const privateECDH = await crypto.subtle.importKey(
    "jwk",
    privateIdentity.ecdhPrivateKey,
    { name: "ECDH", namedCurve: "P-384" },
    false,
    ["deriveBits"]
  );
  const ephemeralPublicKey = JSON.parse(base64URLToString(envelope.ecdh_ephemeral_public_key)) as JsonWebKey;
  const publicECDH = await crypto.subtle.importKey("jwk", ephemeralPublicKey, { name: "ECDH", namedCurve: "P-384" }, false, []);
  const ecdhSecret = await crypto.subtle.deriveBits({ name: "ECDH", public: publicECDH }, privateECDH, 384);
  const salt = base64URLToBytes(envelope.salt);
  const sharedMaterial = concatBytes(kemSecret, new Uint8Array(ecdhSecret));
  const coreEnvelope = envelopeCore(envelope);
  const metadataKey = await deriveAESKey(sharedMaterial, salt, await kdfInfo("metadata-key", coreEnvelope), ["decrypt"]);
  const fileKey = await deriveAESKey(sharedMaterial, salt, await kdfInfo("file-key", coreEnvelope), ["decrypt"]);
  const metadataPlaintext = await crypto.subtle.decrypt(
    {
      name: "AES-GCM",
      iv: bufferSource(base64URLToBytes(envelope.metadata_nonce)),
      additionalData: aad("metadata", coreEnvelope),
      tagLength: 128
    },
    metadataKey,
    bufferSource(base64URLToBytes(envelope.encrypted_metadata))
  );
  const metadata = JSON.parse(textDecoder.decode(metadataPlaintext)) as FileMetadata;
  const plaintext = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: bufferSource(base64URLToBytes(envelope.file_nonce)), additionalData: aad("file", coreEnvelope), tagLength: 128 },
    fileKey,
    ciphertext
  );
  if (plaintext.byteLength !== metadata.size) {
    throw new Error("Decrypted file size does not match metadata");
  }
  const blob = new Blob([plaintext], { type: metadata.content_type || "application/octet-stream" });
  return {
    name: metadata.name || "download",
    type: blob.type,
    size: metadata.size,
    blob
  };
}

function parseEnvelope(raw: string): E2EEnvelope {
  const envelope = JSON.parse(raw) as E2EEnvelope;
  if (envelope.version !== VERSION || envelope.algorithm !== E2E_ALGORITHM) {
    throw new Error("Encrypted upload uses an unsupported E2E envelope");
  }
  return envelope;
}

function envelopeCore(envelope: E2EEnvelope) {
  return {
    version: envelope.version,
    algorithm: envelope.algorithm,
    public_key_fingerprint: envelope.public_key_fingerprint,
    kem_ciphertext: envelope.kem_ciphertext,
    ecdh_ephemeral_public_key: envelope.ecdh_ephemeral_public_key,
    salt: envelope.salt,
    file_nonce: envelope.file_nonce,
    metadata_nonce: envelope.metadata_nonce
  };
}

async function publicKeyFingerprint(publicIdentity: Omit<E2EPublicIdentity, "fingerprint">): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", textEncoder.encode(canonicalJSONString(publicIdentity)));
  return `sha256:${bytesToBase64URL(new Uint8Array(digest))}`;
}

async function deriveAESKey(sharedMaterial: Uint8Array, salt: Uint8Array, info: Uint8Array, keyUsages: KeyUsage[]): Promise<CryptoKey> {
  const material = await crypto.subtle.importKey("raw", bufferSource(sharedMaterial), "HKDF", false, ["deriveKey"]);
  return crypto.subtle.deriveKey(
    { name: "HKDF", hash: "SHA-512", salt: bufferSource(salt), info: bufferSource(info) },
    material,
    { name: "AES-GCM", length: 256 },
    false,
    keyUsages
  );
}

function aad(purpose: string, coreEnvelope: object): Uint8Array<ArrayBuffer> {
  return bufferSource(textEncoder.encode(`Sprag E2E ${purpose} v1\0${canonicalJSONString(coreEnvelope)}`));
}

async function kdfInfo(purpose: string, coreEnvelope: object): Promise<Uint8Array<ArrayBuffer>> {
  const digest = await crypto.subtle.digest("SHA-512", textEncoder.encode(canonicalJSONString(coreEnvelope)));
  return concatBytes(textEncoder.encode(`Sprag E2E ${purpose} v1\0`), new Uint8Array(digest));
}

function canonicalJSONString(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map(canonicalJSONString).join(",")}]`;
  }
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonicalJSONString(record[key])}`)
    .join(",")}}`;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function bytesToBase64URL(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function base64URLToBytes(value: string): Uint8Array<ArrayBuffer> {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
  const binary = atob(padded);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

function bufferSource(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(bytes.byteLength);
  out.set(bytes);
  return out;
}

function stringToBase64URL(value: string): string {
  return bytesToBase64URL(textEncoder.encode(value));
}

function base64URLToString(value: string): string {
  return textDecoder.decode(base64URLToBytes(value));
}
