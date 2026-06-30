#!/usr/bin/env bash
# A fake "repair agent" that reads the prompt but changes nothing, so the verify
# rerun keeps failing. Used to exercise the BLOCKED path of `lf loop`: set
# `repair.agent: fake_noop` in .lunarforge.yml, break src/hello.txt, then run
# `lf loop` — repair attempts run, verification stays red, explain is skipped.
set -euo pipefail

cat >/dev/null || true
echo "fake-repair-noop: read the prompt, made no changes"
