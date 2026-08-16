#!/usr/bin/env bash
# LunarForge's own verify ritual (macOS/Linux) — this repo dogfooding itself.
# `lf verify` and the GitHub Actions workflow both run this script, so the local
# gate and the remote backup check exactly the same things.
set -euo pipefail

echo "==> gofmt"
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "these files are not gofmt-clean:" >&2
  echo "$unformatted" >&2
  echo "fix with: gofmt -w ." >&2
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go build"
go build ./...

echo "==> go test"
go test ./...

echo "verify.sh: all checks passed"
