## Summary

- 

## Scope Check

- [ ] This keeps Sprag focused on one-way secure intake.
- [ ] This does not add uploader accounts, listings, folders, comments, chat, or general collaboration semantics.

## Security / Privacy Check

- [ ] I considered effects on upload slugs, PINs, receipts, E2E envelopes, S3 keys, Tor/onion mode, IP metadata, rate limiting, and logs.
- [ ] Security wording is threat-model accurate and does not overstate anonymity or browser-delivered E2E.
- [ ] No secrets, `.env` files, private keys, uploaded files, local databases, or generated key backups are committed.

## Verification

- [ ] `go test ./...`
- [ ] `cd frontend && npm test`
- [ ] `scripts/check-launch-readiness.sh`
- [ ] Documentation updated when behavior or guarantees changed.
