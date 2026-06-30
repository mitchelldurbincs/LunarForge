#!/usr/bin/env bash
# A fake, dependency-free "repair agent" used by the fixture and tests so
# `lf repair` works end-to-end without Claude/Codex installed.
#
# LunarForge writes the generated repair prompt to this script's STDIN (the same
# way `claude --print` and `codex exec -` read a prompt). A real agent would read
# that prompt, inspect the repo, and make the smallest fix. Here we apply the one
# known fix: write the expected marker text into src/hello.txt so verification
# passes on the rerun.
#
# It deliberately does NOT claim success — only `lf verify` decides that.
set -euo pipefail

# Drain stdin (the prompt) so the writer doesn't get a broken pipe.
cat >/dev/null || true

echo "hello lunarforge" > src/hello.txt
echo "fake-repair-success: wrote expected text to src/hello.txt"
