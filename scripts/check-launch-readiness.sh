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
require_file docs/release-notes/v1.5.1.md
require_file docs/launch/github-repo-settings.md
require_file assets/demo/README.md
require_file assets/demo/sprag-intake-demo.gif

require_text README.md 'Deploy Sprag'
require_text README.md 'Use it for sensitive intake'
require_text README.md 'What Sprag is not'
require_text README.md '![Sprag intake demo](assets/demo/sprag-intake-demo.gif)'
require_text README.md 'docker compose up --build -d'
require_text README.md 'SECURITY.md'
require_text README.md 'CONTRIBUTING.md'
require_text SECURITY.md 'security@sprag.org'
require_text SECURITY.md 'Browser-delivered cryptography boundary'
require_text CONTRIBUTING.md 'Keep Sprag tiny and legible'
require_text docs/release-notes/v1.5.1.md 'Sprag v1.5.1'
require_text docs/launch/github-repo-settings.md 'secure-document-intake'
require_text assets/demo/README.md 'admin creates an intake page'

printf 'launch-readiness check passed\n'
