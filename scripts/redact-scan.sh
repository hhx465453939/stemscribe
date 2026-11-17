#!/usr/bin/env bash
# Open-source redaction scan (AC-14).
#
# Anything matching here must never reach the repository: real hosts, private
# paths, credentials, cloud resource names. The patterns are intentionally broad;
# a false positive costs a minute, a leak costs a history rewrite.
set -uo pipefail

fail=0
scan() {
  local name="$1" pattern="$2"
  # Exclude vendored/generated trees and this script itself.
  local hits
  hits=$(grep -rInE "$pattern" \
      --include='*.go' --include='*.ts' --include='*.tsx' --include='*.js' \
      --include='*.mjs' --include='*.json' --include='*.yml' --include='*.yaml' \
      --include='*.md' --include='*.ps1' --include='*.sh' \
      . 2>/dev/null \
    | grep -vE '^\./(node_modules|frontend/node_modules|frontend/\.next|frontend/out)/' \
    | grep -vE '^\./\.git/' \
    | grep -vE '(pnpm-lock\.yaml|package-lock\.json|yarn\.lock)' \
    | grep -vE 'scripts/redact-scan\.sh' \
    || true)
  if [ -n "$hits" ]; then
    printf 'HIT [%s]:\n%s\n\n' "$name" "$hits"
    fail=1
  fi
}

# Require all four octets: a looser pattern matched version strings such as
# "espree@10.4.0" in lockfiles and produced nothing but false positives.
scan "private IPv4" '\b(10(\.[0-9]{1,3}){3}|192\.168(\.[0-9]{1,3}){2}|172\.(1[6-9]|2[0-9]|3[01])(\.[0-9]{1,3}){2})\b'
# Loopback is deliberately NOT scanned: 127.0.0.1 leaks nothing and appears in
# legitimate local-endpoint documentation, where a false positive hides real hits.
scan "windows user path"   'C:\\\\Users\\\\[A-Za-z0-9_.-]+'
scan "windows user path 2" 'C:/Users/[A-Za-z0-9_.-]+'
scan "bearer token"        'Bearer [A-Za-z0-9._-]{20,}'
scan "openai style key"    'sk-[A-Za-z0-9]{16,}'
scan "aws access key"      'AKIA[0-9A-Z]{16}'
scan "private key block"   'BEGIN (RSA|OPENSSH|EC|PGP) PRIVATE KEY'
scan "generic secret"      '(api[_-]?key|secret|token)[[:space:]]*[:=][[:space:]]*["'"'"'][A-Za-z0-9._-]{24,}'

if [ "$fail" -ne 0 ]; then
  echo "redaction scan FAILED"
  exit 1
fi
echo "redaction scan passed (0 hits)"
