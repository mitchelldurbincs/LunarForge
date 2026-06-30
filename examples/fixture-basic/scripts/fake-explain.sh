#!/usr/bin/env bash
# A fake, dependency-free "explain agent" used by the fixture so `lf explain`
# works end-to-end without Claude/Codex installed.
#
# LunarForge invokes the explain command with the generated prompt as the final
# argument: ./scripts/fake-explain.sh "<prompt>". A real agent would read that
# prompt and produce a review. Here we just echo a canned explanation and a
# short prompt excerpt so you can see the wiring works.
set -euo pipefail

PROMPT="${1:-}"

cat <<EOF
# LunarForge explanation (fake agent)

This explanation was produced by examples/fixture-basic/scripts/fake-explain.sh,
a stand-in for a real explanation agent (e.g. \`claude --print\`).

A real agent would summarize the diff, list changed files, assess the
verification evidence, and flag risks. The full prompt it received begins:

EOF

# Print the first few lines of the prompt to prove it was passed through.
printf '%s\n' "$PROMPT" | head -n 5
