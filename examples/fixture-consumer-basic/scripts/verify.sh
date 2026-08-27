#!/usr/bin/env bash
# Dependency-free verify ritual for the consumer fixture: it asserts that
# src/hello.txt still contains the expected marker text. This stands in for a
# real lint/build/test command so the fixture works without Node/CMake/etc.
#
# The point of this fixture is to prove that a normal project (one WITHOUT
# LunarForge's ./cmd/lf source) can still run `lf ci` and generate a
# consumer-safe workflow via `lf gen-actions --install-mode go-install`.
set -euo pipefail

EXPECTED="hello consumer"
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
