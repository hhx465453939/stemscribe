#!/usr/bin/env bash
# Contract gate: CHG three-piece + required docs + JSON schemas.
# Mirrors the `contract` job in .github/workflows/ci.yml so it can be run locally
# before pushing.
set -uo pipefail

fail=0
say() { printf '%s\n' "$*"; }
err() { printf 'FAIL: %s\n' "$*"; fail=1; }

say "== CHG three-piece =="
for f in tasks/CHG-0001/SPEC.md tasks/CHG-0001/PRD.md tasks/CHG-0001/TRACEABILITY.md; do
  if [ -s "$f" ]; then say "ok   $f"; else err "missing or empty: $f"; fi
done
if grep -qiE '^\| *status *\| *\**draft' tasks/CHG-0001/SPEC.md; then
  err "CHG-0001 is still draft"
else
  say "ok   CHG-0001 status is not draft"
fi
ac_count=$(grep -cE '^\| AC-[0-9]+' tasks/CHG-0001/SPEC.md || true)
if [ "${ac_count:-0}" -lt 12 ]; then err "expected >= 12 acceptance criteria, found ${ac_count:-0}"; else say "ok   ${ac_count} acceptance criteria"; fi

say "== required docs =="
for f in docs/ARCHITECTURE.md docs/PIPELINE.md docs/MODEL-REGISTRY.md \
         docs/RESOURCE-SCHEDULER.md docs/LLM-PROVIDERS.md docs/UI-DESIGN.md \
         docs/DECISIONS.md docs/ROADMAP.md docs/UPSTREAM-RESEARCH.md; do
  if [ -s "$f" ]; then say "ok   $f"; else err "missing or empty: $f"; fi
done

say "== ADRs =="
adr_count=$(grep -cE '^## ADR-00[0-9]' docs/DECISIONS.md || true)
if [ "${adr_count:-0}" -lt 8 ]; then err "expected >= 8 ADRs, found ${adr_count:-0}"; else say "ok   ${adr_count} ADRs"; fi
for k in 决策 理由 代价 被否决方案; do
  n=$(grep -c "^\*\*$k\*\*" docs/DECISIONS.md || true)
  if [ "${n:-0}" -lt 8 ]; then err "ADR section '$k' appears ${n:-0} times, want >= 8"; fi
done

say "== JSON schemas =="
# Resolve a *working* interpreter: on some Windows setups `python3` resolves to a
# broken Store alias that `command -v` finds but which fails on execution, which
# made valid schemas look invalid.
PY=""
for cand in python3 python; do
  if command -v "$cand" >/dev/null 2>&1 && "$cand" -c 'import sys' >/dev/null 2>&1; then
    PY="$cand"
    break
  fi
done
for f in docs/schema/*.json; do
  [ -e "$f" ] || continue
  if [ -n "$PY" ]; then
    if "$PY" -c "import json,sys; json.load(open(sys.argv[1],encoding='utf-8'))" "$f" 2>/dev/null; then
      say "ok   $f"
    else
      err "invalid json: $f"
    fi
  else
    say "skip $f (no python interpreter available)"
  fi
done

if [ -n "$PY" ]; then
  if "$PY" scripts/schema-check.py; then
    :
  else
    rc=$?
    if [ "$rc" -eq 2 ]; then
      say "skip instance validation (jsonschema not installed)"
    else
      err "registry.json does not conform to its schema"
    fi
  fi
else
  say "skip instance validation (no python interpreter available)"
fi

# The closure that matters: until this ran, registry.json could violate its own
# contract forever, because only the schema file itself was ever parsed.
if [ "$fail" -ne 0 ]; then
  say ""
  say "contract check FAILED"
  exit 1
fi
say ""
say "contract check passed"
