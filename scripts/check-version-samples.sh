#!/usr/bin/env bash
# Fail if a documented "gitcollect vX.Y.Z os/arch" sample disagrees with the
# version being released.
#
# These samples have drifted three times. Twice they were merely stale; once
# the sample read v1.0.0, which is exactly what you get when you install
# without the /v3 module suffix — so the page confirmed the very failure it
# warned about two sections earlier. Correcting them by hand after each
# release is what kept failing, so the release gates on them instead.
#
# Only the "expected output of gitcollect version" shape is checked. Prose
# that names an older version on purpose — the get-update walkthrough
# upgrading v3.2.0 to 3.3.0, or the "requires v3.2.0 or newer" note — is
# left alone, because those are correct precisely by not being current.
set -euo pipefail

want="${1:-${GITHUB_REF_NAME:-}}"
if [ -z "$want" ]; then
  echo "usage: $0 <version>   (e.g. v3.3.0)" >&2
  exit 2
fi
want="${want#v}"

files=(README.md MANUAL.md docs/index.html docs/commands.html)

# Two shapes claim to be the running version: the platform line printed by
# "gitcollect version", and get-update's up-to-date message. Both go stale
# the same way, so both are checked.
pattern='gitcollect v[0-9]+\.[0-9]+\.[0-9]+ ((linux|darwin|windows)/[a-z0-9]+|is the latest release)'

fail=0
found=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  while IFS= read -r hit; do
    found=$((found + 1))
    line="${hit%%:*}"
    text="${hit#*:}"
    got="$(printf '%s' "$text" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
    if [ "${got#v}" != "$want" ]; then
      echo "::error file=$f,line=$line::version sample says ${got} but the release is v${want} — update it"
      echo "  $f:$line: $text" >&2
      fail=1
    fi
  done < <(grep -nEo "$pattern" "$f" || true)
done

if [ "$found" -eq 0 ]; then
  echo "::warning::no version samples found — the check may be looking in the wrong place"
fi

if [ "$fail" -ne 0 ]; then
  echo >&2
  echo "Version samples must match the tag being released. Update them in the" >&2
  echo "commit you tag, so a released page never advertises a version that is" >&2
  echo "not the one shipping." >&2
  exit 1
fi

echo "version samples agree with v${want} (${found} checked)"
