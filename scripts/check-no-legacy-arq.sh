#!/usr/bin/env bash
# Copyright (c) 2026 Scantr LLC. All rights reserved.
# Elevarq is a trade name of Scantr LLC.
#
# De-arq guard (#2568): a baseline ratchet that blocks NEW legacy "arq"
# naming (the pre-rebrand product name) while the remaining rename debt is
# burned down.
#
# The product is Elevarq; legacy standalone "arq" must not reappear in
# runtime strings (container names, /var/lib/arq, arq-license.json, image
# refs, helm chart dirs), code identifiers, or live docs.
#
# Detection: case-insensitive "arq" at a WORD BOUNDARY (\barq). This is the
# whole trick that makes de-arq reliable — a word boundary cannot fall inside
# "elev|arq" (the "a" is preceded by the word char "v"), so `\barq` matches
# standalone `arq` / `arq-*` / `/…/arq` but NEVER matches `elevarq`. It also
# never matches `pgagroal` (no "arq" at all).
#
# Modes:
#   check   (default) — fail if any file's legacy-arq count EXCEEDS the
#                       committed baseline, or a NEW file appears. This is
#                       the CI gate: new `arq` can never land.
#   list             — print the current per-file inventory (path:count).
#   update           — regenerate the baseline (run after you fix a file, or
#                       after a legitimate, allow-listed addition).
#
# Burn the baseline down to empty to complete the rename; the guard then
# permanently forbids any `arq` at all.
#
# Scope decision (#2568, product owner): runtime + code/docs, NOT GitHub repo
# renames. History and immutable files are excluded below.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

BASELINE="scripts/de-arq-baseline.txt"
SELF="scripts/check-no-legacy-arq.sh"

# The scan uses `git grep`, which is present on every dev machine and CI runner
# (no ripgrep dependency) and searches only TRACKED files (so build output and
# gitignored trees are excluded for free). `-I` skips binary files; `-P` selects
# PCRE (lookbehind + word boundary). git built with PCRE is standard on Linux
# CI and modern macOS.

# Excluded surfaces (allow-list): history + immutable + the guard's own files,
# expressed as git pathspecs. GitHub repo names (Arq, Arq-Signals,
# Arq-Workbench) are out of scope and are NOT renamed, so references that
# resolve to a real repo/module path are expected to remain until a future
# repo-rename program.
EXCLUDES=(
  ':!CHANGELOG.md'
  ':!LICENSE'
  ':!NOTICE'
  ":!$BASELINE"
  ":!$SELF"
  ':!**/CHANGELOG.md'
)

# scan prints "path:count" (rg -c) for every tracked file with >=1 legacy-arq
# match, sorted. rg honours .gitignore and skips binary files.
# The pattern is case-insensitive `arq` at a word boundary, EXCEPT when it is
# the anti-arq vocabulary itself — "de-arq" / "legacy-arq" (and thus
# "no-legacy-arq"). Those are the tooling's own terms, not the legacy product
# name, so the guard must not count them as debt anywhere.
PATTERN='(?i)(?<!de-)(?<!legacy-)\barq'

scan() {
  git grep -I -c -P "$PATTERN" -- "${EXCLUDES[@]}" 2>/dev/null \
    | LC_ALL=C sort || true
}

case "${1:-check}" in
  list)
    scan
    ;;
  update)
    scan > "$BASELINE"
    files=$(wc -l < "$BASELINE" | tr -d ' ')
    lines=$(awk -F: '{s+=$NF} END{print s+0}' "$BASELINE")
    echo "de-arq baseline updated: $files files, $lines legacy-arq lines"
    ;;
  check)
    # Compare current scan against the baseline with awk (portable to macOS
    # bash 3.2 — no associative arrays). The count is the LAST colon-field, so
    # paths that themselves contain a colon still parse correctly.
    rc=0
    scan | awk -F: -v base="$BASELINE" '
      BEGIN {
        while ((getline line < base) > 0) {
          n = split(line, a, ":")
          cnt = a[n]
          key = substr(line, 1, length(line) - length(cnt) - 1)
          b[key] = cnt
        }
      }
      {
        cnt = $NF + 0
        key = substr($0, 1, length($0) - length($NF) - 1)
        if (!(key in b)) { print "NEW legacy-arq file: " key " (" cnt ")"; f = 1 }
        else if (cnt > b[key] + 0) { print "MORE legacy-arq in " key ": " cnt " (baseline " b[key] ")"; f = 1 }
      }
      END { exit f ? 1 : 0 }
    ' || rc=$?
    if [ "$rc" -ne 0 ]; then
      echo ""
      echo "de-arq guard FAILED: new legacy 'arq' was introduced."
      echo "Rename it to 'elevarq'. If the addition is legitimate and allow-listed,"
      echo "record it with:  bash $SELF update"
      exit 1
    fi
    echo "de-arq guard OK: no new legacy 'arq' beyond the baseline ($BASELINE)."
    echo "Burn the baseline to zero to finish the rename."
    ;;
  *)
    echo "usage: $SELF [check|list|update]" >&2
    exit 2
    ;;
esac
