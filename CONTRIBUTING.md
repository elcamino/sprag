# Contributing to Sprag

Thanks for considering a contribution. Sprag is intentionally narrow: one-way secure file intake for people who should not need an account, folder, listing, download path, or collaboration workspace.

## Keep Sprag tiny and legible

Good changes usually deepen one of these properties:

- one-way intake isolation
- self-hosted deployment simplicity
- server-blind E2E intake
- evidence-grade handling records
- metadata minimization
- Tor/onion deployment clarity
- professional sensitive-intake workflows for lawyers, journalists, HR, compliance, doctors, and researchers

Changes are likely out of scope when they turn Sprag into a Dropbox, ticketing system, chat tool, form builder, multi-tenant workspace, or general document portal.

## Development Setup

Run backend tests from the repository root:

```bash
go test ./...
```

Run frontend tests from the frontend directory:

```bash
cd frontend
npm test
```

Run the launch-readiness documentation check from the repository root:

```bash
scripts/check-launch-readiness.sh
```

## Pull Request Checklist

Before opening a pull request:

- keep the change focused on one problem
- update `README.md`, `INSTALL.md`, `docs/features.md`, or `docs/sprag-prd.md` when behavior or guarantees change
- keep security wording threat-model accurate
- add or update tests for behavior changes
- run the relevant Go and frontend tests
- avoid committing secrets, `.env`, uploaded files, local databases, generated keys, or browser private-key backups

## Security-Sensitive Changes

For changes touching authentication, upload slugs, PINs, receipts, E2E envelopes, S3 keys, Tor/onion mode, IP metadata, rate limiting, or logging, describe the attacker model in the pull request. A change that sounds privacy-positive but widens metadata exposure should be treated as a regression until proven otherwise.
