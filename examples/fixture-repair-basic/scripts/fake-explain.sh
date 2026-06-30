#!/usr/bin/env bash
# A fake, dependency-free "explain agent" so `lf explain` works end-to-end in the
# fixture without Claude/Codex installed. LunarForge passes the prompt as the
# final argument; here we echo a canned explanation plus a short prompt excerpt.
set -euo pipefail

PROMPT="${1:-}"

cat <<EOF
# LunarForge explanation (fake agent)

This explanation was produced by a stand-in for a real explanation agent.
A real agent would summarize the diff, list changed files, assess the
verification evidence, and flag risks. The prompt it received begins:

EOF

printf '%s\n' "$PROMPT" | head -n 5
