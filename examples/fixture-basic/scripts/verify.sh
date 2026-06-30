#!/usr/bin/env bash
# Dependency-free verify ritual for the fixture: it asserts that
# src/hello.txt still contains the expected marker text. This stands in for a
# real lint/build/test command so the fixture works without Node/CMake/etc.
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
exit 1
