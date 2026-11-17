#!/usr/bin/env bash
# CHG-0007 guard. Three independent checks, each with a single responsibility:
#
#   tokens    every theme defines the full semantic token set
#   parity    zh and en carry exactly the same key set (type checker is primary,
#             this catches accidental edits that still type-check, e.g. removals)
#   literal   no component hardcodes user-visible CJK text
#
# Exit non-zero on any failure so CI can gate on it.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# Every run leaves a log next to the script: months later this is the only
# reproducible evidence of what passed and when (W4).
LOG="scripts/i18n-check.log"
exec > >(tee "$LOG") 2>&1
printf '== i18n-check %s ==\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

CSS=frontend/app/globals.css
I18N=frontend/lib/i18n.ts
fail=0

err() { printf 'FAIL: %s\n' "$*"; fail=1; }
ok() { printf 'ok   %s\n' "$*"; }

check_tokens() {
  local required=(--bg --surface --surface-hi --surface-lit
                  --text-1 --text-2 --text-3
                  --line --line-hi --lamp --lamp-fill --lamp-ink --fail --glow)

  # Extract each theme block by its selector and closing brace.
  local light dark
  light=$(awk '/^:root,$/,/^}/' "$CSS"; awk '/^:root\[data-theme="light"\]/,/^}/' "$CSS")
  dark=$(awk '/^:root\[data-theme="dark"\]/,/^}/' "$CSS")

  local t missing=0
  for t in "${required[@]}"; do
    if ! grep -q -- "$t:" <<<"$light"; then err "light theme missing $t"; missing=1; fi
    if ! grep -q -- "$t:" <<<"$dark"; then err "dark theme missing $t"; missing=1; fi
  done
  [ "$missing" -eq 0 ] && ok "tokens: ${#required[@]} present in both themes"
}

check_parity() {
  local zh en
  zh=$(sed -n '/^export const zh = {/,/^} as const;/p' "$I18N" | grep -cE '^  "[^"]+":')
  en=$(sed -n '/^export const en: Record<Key, string> = {/,/^};/p' "$I18N" | grep -cE '^  "[^"]+":')
  if [ "$zh" -eq 0 ] || [ "$en" -eq 0 ]; then
    err "could not count keys (zh=$zh en=$en)"
    return
  fi
  if [ "$zh" -ne "$en" ]; then
    err "key count differs: zh=$zh en=$en"
    return
  fi
  ok "parity: $zh keys in both locales"
}

check_literal() {
  local hits
  hits=$(grep -rnE '[一-鿿]' frontend/app --include='*.tsx' \
    | grep -v 'i18n-allow' \
    | grep -vE '^\S+:[0-9]+: *(//|\*|/\*)' \
    | grep -vE '\bt\(' \
    || true)
  if [ -n "$hits" ]; then
    printf '%s\n' "$hits"
    err "hardcoded CJK in components (use t(), or annotate with i18n-allow)"
    return
  fi
  ok "literal: no hardcoded CJK in frontend/app/**/*.tsx"
}

case "${1:-all}" in
  tokens) check_tokens ;;
  parity) check_parity ;;
  literal) check_literal ;;
  all) check_tokens; check_parity; check_literal ;;
  *) echo "usage: $0 [tokens|parity|literal|all]"; exit 2 ;;
esac

if [ "$fail" -ne 0 ]; then
  echo ""
  echo "i18n/theme check FAILED"
  exit 1
fi
echo ""
echo "i18n/theme check passed"
