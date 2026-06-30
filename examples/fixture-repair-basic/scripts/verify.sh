#!/usr/bin/env bash
# Dependency-free verify ritual for the repair fixture: it asserts that
# src/hello.txt contains the expected marker text. The fixture ships in a
# FAILING state (the file contains "broken"); a repair agent must fix it.
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
