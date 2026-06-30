#!/usr/bin/env bash
# A fake "repair agent" that exits non-zero (as if the agent itself errored).
# Used to exercise the agent-failure path. It still drains stdin first.
set -euo pipefail

cat >/dev/null || true
echo "fake-repair-fail: simulated agent failure" >&2
exit 1
