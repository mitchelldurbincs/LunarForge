#!/usr/bin/env bash
# A fake "repair agent" that reads the prompt but changes nothing, so the verify
# rerun keeps failing. Used to exercise the max-attempts exhaustion path.
set -euo pipefail

cat >/dev/null || true
echo "fake-repair-noop: read the prompt, made no changes"
