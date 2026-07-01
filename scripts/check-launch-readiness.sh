#!/bin/sh
set -eu

fail() {
  printf 'launch-readiness check failed: %s\n' "$1" >&2
  exit 1
}

require_file() {
  file="$1"
  [ -f "$file" ] || fail "missing $file"
}

require_text() {
  file="$1"
  needle="$2"
  grep -Fq "$needle" "$file" || fail "$file does not contain: $needle"
}

require_file README.md
require_file INSTALL.md
require_file LICENSE.md
require_file SECURITY.md
require_file CONTRIBUTING.md
require_file .github/ISSUE_TEMPLATE/bug_report.yml
require_file .github/ISSUE_TEMPLATE/feature_request.yml
require_file .github/PULL_REQUEST_TEMPLATE.md
require_file assets/demo/sprag-upload.webp
require_file assets/demo/sprag-admin.webp

require_text README.md 'Deploy Sprag'
require_text README.md 'Use it for sensitive intake'
require_text README.md 'What Sprag is not'
require_text README.md '![Sprag uploader view — submitting files to an intake page](assets/demo/sprag-upload.webp)'
require_text README.md '![Sprag admin dashboard — reviewing submissions on an intake page](assets/demo/sprag-admin.webp)'
require_text README.md 'docker compose up --build -d'
require_text README.md 'SECURITY.md'
require_text README.md 'CONTRIBUTING.md'
require_text SECURITY.md 'security@sprag.org'
require_text SECURITY.md 'Browser-delivered cryptography boundary'
require_text CONTRIBUTING.md 'Keep Sprag tiny and legible'

printf 'launch-readiness check passed\n'
