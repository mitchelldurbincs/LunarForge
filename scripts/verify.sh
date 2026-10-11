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

# The optional lint gate supports both installed major versions. Once available,
# lint findings and tool failures fail verification; only an absent tool skips.
if command -v golangci-lint >/dev/null 2>&1; then
  echo "==> funlen, gocognit, lll"
  lint_version="$(golangci-lint version)"
  case "$lint_version" in
    *" version v1."*|*" version 1."*) lint_config="scripts/golangci-v1.yml" ;;
    *" version v2."*|*" version 2."*) lint_config=".golangci.yml" ;;
    *) echo "unsupported golangci-lint version: $lint_version" >&2; exit 1 ;;
  esac
  golangci-lint run --config "$lint_config" ./...
else
  echo "==> skipping funlen, gocognit, lll (golangci-lint not installed)"
fi

echo "verify.sh: all checks passed"
