# Security Policy

Sprag is a small, self-hosted secure intake box. Security reports are welcome, especially when they affect one-way intake isolation, uploader anonymity expectations, server-blind E2E mode, admin authentication, S3 object handling, receipt URLs, Tor/onion deployment, or evidence manifests.

## Supported Versions

Security fixes target the current `main` branch and the latest tagged release. Older tags are kept for source history, but they are not maintained as separate long-term support branches.

## Reporting a Vulnerability

Email security reports to security@sprag.org.

Please include:

- the affected Sprag version or commit hash
- the deployment mode: plaintext, E2E optional, E2E required, or onion-only
- clear reproduction steps
- observed impact
- whether the issue requires an attacker to control the Sprag host, S3 bucket, network path, admin browser, uploader browser, or only a public upload URL

Do not open a public GitHub issue for an exploitable vulnerability before there is a coordinated fix or mitigation.

## Expected Response

You should receive an initial response within 72 hours. Confirmed vulnerabilities will be handled with a private fix branch when needed, a public patch release, and release notes that describe impact without publishing weaponized instructions.

## Security Model Boundary

Sprag's E2E mode is server-blind for passive at-rest compromise: the server and S3 bucket store ciphertext and opaque envelopes, not plaintext file bodies or original filenames.

Browser-delivered cryptography boundary: Sprag cannot protect an uploader from a host that is actively malicious at upload time and deliberately serves modified JavaScript or swaps the page public key. That boundary is documented because overstating browser E2E would make the project less trustworthy.

Clearnet deployment does not guarantee sender anonymity. For stronger source anonymity, use onion-only ingress and set `ANONYMOUS_INGRESS=true`; then still assume the admin, hosting provider, and endpoint devices remain part of the threat model.
