#!/usr/bin/env bash
# Dependency-free verify ritual for the loop fixture: it asserts that
# src/hello.txt contains the expected marker text. The fixture ships PASSING
# (the file already contains the marker). To see `lf loop` repair, break it:
#
#   echo broken > src/hello.txt
#
# then run `lf loop` — the configured fake repair agent rewrites the marker.
set -euo pipefail

EXPECTED="hello lunarforge"
FILE="src/hello.txt"

if [ ! -f "$FILE" ]; then
  echo "verify: $FILE is missing" >&2
  exit 1
fi

if grep -q "$EXPECTED" "$FILE"; then
  echo "verify: $FILE contains expected text: \"$EXPECTED\""
  exit 0
fi

echo "verify: $FILE does not contain expected text: \"$EXPECTED\"" >&2
echo "verify: found instead:" >&2
cat "$FILE" >&2
exit 1
