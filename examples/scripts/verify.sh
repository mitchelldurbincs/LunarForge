#!/usr/bin/env bash
# Example repo verify ritual for macOS/Linux.
# This is the "real repo ritual" LunarForge enforces. Edit it to run the
# lint/build/test commands that actually matter for your project, and make it
# exit non-zero on any failure.
set -euo pipefail

echo "==> lint"
# npm run lint

echo "==> typecheck"
# npm run typecheck

echo "==> test"
# npm test

echo "==> build"
# npm run build

echo "verify.sh: all checks passed"
