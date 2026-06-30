# LunarForge (`lf`)

**A local-first engineering gate for AI-assisted coding.**

You drive an AI agent (Claude Code, Codex, or another). The agent edits files.
LunarForge is the layer that actually **runs your repo's lint/build/test
ritual**, **records evidence tied to the exact current git diff**, and
**explains what changed** — so you review from a much better place.

The core rules LunarForge enforces locally:

```
No fresh evidence        → not ready.
Build/test/lint failed   → not ready.
Diff changed after verify → evidence is stale.
```

---

## The philosophy

```
AGENTS.md / CLAUDE.md = reminders for agents
scripts/verify.sh     = real repo ritual
lf verify             = local proof
lf status             = fresh/stale proof check
pre-push hook         = local enforcement before pushing
lf explain            = diff understanding
GitHub Actions        = remote backup later
```

LunarForge does **not** replace Claude Code, Codex, or manual driving. It is the
**local evidence layer that runs after an AI edits your code**: it proves the
checks passed, ties that proof to the exact code you're about to push, and
blocks the push if the proof is missing, failed, or stale.

### Why AGENTS.md / CLAUDE.md are not enough by themselves

`AGENTS.md` and `CLAUDE.md` are **reminders**. They tell an agent "please run
the tests" or "this repo uses npm." But they are advisory text. Nothing checks
that the agent actually ran anything, nothing records *whether it passed*, and
nothing notices when the code changed again *after* the checks ran.

LunarForge is the **enforcement layer**:

- It runs the commands for real.
- It saves evidence (exit codes, stdout, stderr, timing) on disk.
- It binds that evidence to a hash of the current diff, so if the code changes
  afterward, the evidence is flagged **stale**.
- It can block a `git push` until there is fresh, passing evidence.

Reminders ask. LunarForge verifies.

---

## What LunarForge is

A small, maintainable CLI (`lf`) that does a few things well:

1. **`lf verify`** — runs your configured commands and records evidence.
2. **`lf status`** — tells you if the latest evidence is fresh and passing.
3. **`lf explain`** — explains the current diff using git + the latest evidence.
4. **`lf repair`** — when verification **failed**, asks a configured AI agent for
   the smallest safe fix and reruns `lf verify`. The agent never declares
   success — only `lf verify` can. See [`lf repair` — repair failed gates](#lf-repair).
5. **`lf install-hooks`** — installs a pre-push gate.

## What LunarForge is *not* (yet)

It is intentionally **not** an agent framework. It does **not** do autonomous
implementation from vague tasks, `lf run "build feature"`, multi-agent
workflows, remote servers, dashboards, GitHub Actions generation, or
multi-machine routing. `lf repair` is deliberately narrow: it only reacts to a
**failed** verification run and tries to make that exact gate pass — it does not
do open-ended feature work. See [Roadmap](#roadmap).

The core is: **local verification + evidence + explanation, with an opt-in,
narrow repair of failed gates.**

---

## The intended workflow

```
1. You manually drive Claude Code / Codex / another agent.
2. The agent edits files.
3. You run `lf verify`.
4. LunarForge runs your repo's required local lint/build/test command(s).
5. LunarForge saves evidence tied to the exact current git diff.
6. You optionally run `lf explain`.
7. You review the change.
8. When you push, the pre-push hook runs `lf status --require-fresh-passing`.
9. If evidence is missing, failed, or stale, the push is blocked.
```

**The invariant:** no fresh passing evidence means the repo is not ready to
push.

---

## Install / build locally

LunarForge is a single Go binary with one dependency (`gopkg.in/yaml.v3`).

```bash
# Requires Go 1.24+
git clone <this-repo>
cd lunarforge

# Build a local binary
go build -o lf ./cmd/lf

# Or install onto your PATH
go install ./cmd/lf      # installs `lf` into $(go env GOBIN) or $GOPATH/bin
```

Put `lf` somewhere on your `PATH`. Verify:

```bash
lf version
lf help
```

It is cross-platform: on macOS/Linux verify commands run through `sh -c`, on
Windows through `cmd /C`. The pre-push hook is a POSIX `sh` script (git ships
its own `sh` on Windows).

---

## Day one

```bash
cd your-repo
lf init                 # creates .lunarforge.yml and .lf/
# edit .lunarforge.yml, write scripts/verify.sh (or verify.ps1)
lf verify               # run checks, save evidence
lf status               # is evidence fresh + passing?
lf explain              # explain the current diff (optional)
lf install-hooks        # block pushes without fresh passing evidence
```

## Recommended daily loop

```bash
# manually drive Claude Code / Codex; the agent edits files
git add -A && git commit -m "..."   # commit the change you want to push
lf verify                           # prove the checks pass for that commit

# if verify failed and you have a repair agent configured:
lf repair                           # ask the agent for the smallest fix, then reverify

lf explain                          # (optional) understand the diff
git push                            # pre-push hook gates on fresh passing evidence
```

The pre-push hook blocks the push unless fresh, passing evidence exists for the
exact code being pushed. Commit first, then `lf verify`, so the evidence is tied
to the commit you push.

---

## Creating `.lunarforge.yml`

LunarForge looks for `.lunarforge.yml` in the current repo (walking up to the
repo root). `lf init` writes a minimal starter that runs a single script:

```yaml
version: 1

project:
  name: example-repo

verify:
  commands:
    - id: verify
      run: ./scripts/verify.sh

explain:
  agent: claude
  command: claude
  args:
    - --print
    - --permission-mode
    - plan

evidence:
  dir: .lf/runs
  require_fresh_diff: true
```

You can list **multiple** verify commands; they run in order and stop on the
first failure (use `lf verify --continue-on-failure` to run them all).

### Example: Node

```yaml
version: 1

project:
  name: node-app

verify:
  commands:
    - id: lint
      run: npm run lint
    - id: typecheck
      run: npm run typecheck
    - id: test
      run: npm test
    - id: build
      run: npm run build

explain:
  agent: claude
  command: claude
  args:
    - --print
    - --permission-mode
    - plan

evidence:
  dir: .lf/runs
  require_fresh_diff: true
```

### Example: C++

```yaml
version: 1

project:
  name: imgui-tool

verify:
  commands:
    - id: build_debug
      run: cmake --build build --config Debug
    - id: test
      run: ctest --test-dir build --output-on-failure
    - id: build_release
      run: cmake --build build --config Release

explain:
  agent: claude
  command: claude
  args:
    - --print
    - --permission-mode
    - plan

evidence:
  dir: .lf/runs
  require_fresh_diff: true
```

Ready-to-copy versions live in [`examples/`](examples/), along with starter
[`verify.sh`](examples/scripts/verify.sh) / [`verify.ps1`](examples/scripts/verify.ps1)
scripts.

---

## How the commands work

### `lf init`

Creates `.lunarforge.yml` (a single-script starter named after the current
directory) and the `.lf/` evidence directory. It will **not** overwrite an
existing `.lunarforge.yml` unless you pass `--force`. It also drops a
`.lf/.gitignore` so run artifacts stay local by default, and reminds you to
create `scripts/verify.sh` / `scripts/verify.ps1`.

### `lf verify`

1. Loads `.lunarforge.yml`.
2. Confirms you're inside a git repo.
3. Computes a **diff hash** of the current changes (see below).
4. Runs each verify command in order, capturing id, command string, start/end
   time, duration, exit code, stdout, stderr, and pass/fail.
5. Stops on the first failure by default (`--continue-on-failure` to override).
6. Saves evidence under `.lf/runs/<timestamp>/` and updates `.lf/latest`, even
   when a command fails.

```
LunarForge verify

✅ lint passed       1.2s
✅ typecheck passed  3.8s
✅ test passed       5.4s
✅ build passed      8.1s

Result:
✅ ready locally

Evidence:
.lf/runs/2026-06-30T14-22-10/evidence.json

Diff:
sha256:abc123...
```

On failure it prints the failing command and points at its logs, still saves
evidence, and exits non-zero:

```
LunarForge verify

✅ lint passed  1.2s
❌ test failed  2.9s

Result:
❌ not ready

Failed command:
npm test

Logs:
.lf/runs/2026-06-30T14-25-03/commands/test.stdout.txt
.lf/runs/2026-06-30T14-25-03/commands/test.stderr.txt
```

#### The diff hash

Evidence is bound to the exact code that would be pushed via a deterministic
SHA-256 of:

```bash
git rev-parse HEAD          # the commit being pushed
git diff --binary           # tracked, unstaged changes
git diff --cached --binary  # staged changes
git status --porcelain      # which files are added/modified/untracked
```

If **HEAD advances** (you make a new commit) or your **tracked/staged**
working-tree changes change after `lf verify`, the hash changes and the evidence
becomes **stale**. LunarForge's own evidence directory (`.lf/`) is excluded from
the hash, so recording evidence never makes that evidence stale.

**Known limitations (by design for the MVP):**

- The *contents* of an **untracked** file are not hashed — an untracked file
  registers only by name via `git status --porcelain`. Track or stage a file to
  have its contents gate the push.
- The hash reflects HEAD plus uncommitted changes, not the full file tree. If
  you `lf verify` a dirty tree and then commit those exact changes, re-run
  `lf verify` so the evidence is tied to the new commit (committing changes the
  hash). The recommended loop — *commit, then verify* — avoids this.

#### Evidence layout

```
.lf/runs/2026-06-30T14-22-10/
  evidence.json          # machine-readable record (below)
  summary.md             # human-readable summary table
  explanation.md         # written by `lf explain`
  explain-prompt.md      # the exact prompt sent to the explain agent
  commands/
    lint.stdout.txt
    lint.stderr.txt
    test.stdout.txt
    test.stderr.txt
.lf/latest               # pointer to the most recent run id
```

`evidence.json` keeps large output out of the JSON by pointing at the
per-command files:

```json
{
  "version": 1,
  "project": "example-repo",
  "run_id": "2026-06-30T14-22-10",
  "started_at": "2026-06-30T14:22:10Z",
  "finished_at": "2026-06-30T14:23:02Z",
  "result": "passed",
  "diff_hash": "sha256:abc123",
  "git": { "branch": "main", "head": "abc1234", "status_porcelain": "..." },
  "commands": [
    {
      "id": "lint",
      "run": "npm run lint",
      "started_at": "...",
      "finished_at": "...",
      "duration_ms": 1234,
      "exit_code": 0,
      "stdout_path": "commands/lint.stdout.txt",
      "stderr_path": "commands/lint.stderr.txt",
      "result": "passed"
    }
  ]
}
```

### `lf status`

This is the core enforcement command. It loads the latest evidence, recomputes
the current diff hash, and reports whether the evidence is **fresh** (matches
the current code) and **passing**.

```
LunarForge status

Latest evidence:
✅ passed

Freshness:
✅ fresh for current diff

Result:
✅ ready to push
```

`lf status --require-fresh-passing` (used by the pre-push hook) makes the exit
code the source of truth. It exits:

- **`0`** only when latest evidence **exists**, **passed**, and its diff hash
  **matches** the current code.
- **non-zero** when any of these hold: no evidence exists, the latest run
  failed, the evidence is stale, the current directory is not a git repo, or
  `.lunarforge.yml` is missing/invalid.

`--strict` is accepted as an alias. `lf status --json` prints the same decision
as machine-readable JSON (`ready`, `reason`, hashes, run id) for scripting.

Example states:

```
Latest evidence:        Latest evidence:        Latest evidence:
❌ none found           ✅ passed               ❌ failed

Result:                 Freshness:              Result:
❌ not ready to push    ⚠️ stale — ...          ❌ not ready to push

Run:                    Result:                 Run:
lf verify               ❌ not ready to push    lf verify
```

### `lf explain`

1. Reads current git status + diff.
2. Loads the latest evidence (if any) and decides fresh vs. stale.
3. Builds a prompt asking for: a concise summary, files changed, why each file
   changed, verification evidence, evidence freshness, risks, and manual review
   suggestions.
4. Invokes the configured explain command using an **exec-style argument
   array** (no fragile shell string). For the config above it runs:

   ```bash
   claude --print --permission-mode plan "<generated prompt>"
   ```

5. Saves the explanation to `.lf/runs/<run>/explanation.md` and prints it.

`lf explain` is **advisory, not a gate** — it is not required by the pre-push
hook, and it works whether evidence is fresh, stale, failed, or missing. The
prompt asks the agent for a concise summary, files changed, why each changed,
verification status, whether evidence is fresh/stale/failed/missing, risks, and
manual review suggestions.

The generated prompt is **always** saved to `.lf/runs/<run>/explain-prompt.md`
first — so if the explain command is missing or fails, you still have the prompt
to run manually (and `lf explain` exits non-zero without aborting your work).

Flags:

- `lf explain --print-prompt` — print the generated prompt and stop (no agent).
- `lf explain --no-run` (alias `--prompt-only`) — save the prompt without
  invoking any agent.

The explain command can be a bare name resolved on `PATH` (e.g. `claude`) or a
repo-relative path (e.g. `./scripts/fake-explain.sh`); relative commands resolve
against the repo root. This makes it easy to wire a fake explain script in CI or
fixtures.

### `lf repair`

**Repair failed gates.** When `lf verify` has **failed**, `lf repair` hands the failure to a configured
AI agent and asks for the **smallest safe fix**, then reruns `lf verify`. It is
not autonomous feature work — it only ever responds to **failed LunarForge
verification evidence**, and the agent does **not** get to declare success. Only
`lf verify` can.

What it does:

```
1. Load .lunarforge.yml and the latest evidence.
2. Refuse if there is no evidence, or if the latest evidence passed.
3. Identify the failed command(s) and read their stdout/stderr logs.
4. Read current git status and git diff.
5. Build a strict repair prompt (saved under the run dir).
6. Invoke the configured repair agent (prompt delivered on stdin).
7. Save the agent's stdout/stderr/result.
8. Rerun `lf verify`.
9. Stop when verify passes, or after max attempts.
```

The prompt is strict by construction: make the smallest safe diff; do not start
unrelated refactors; do not weaken or delete tests; do not skip the failing
command; do not edit `.lunarforge.yml` unless the failure is clearly a config
problem; do not push/commit/branch; do not edit generated/vendor/secret paths;
and after editing, do **not** claim success.

Flags (priority order):

- `lf repair` — repair the latest failed run.
- `lf repair --dry-run` — show the plan (agent command + where artifacts would
  be written) without invoking the agent or running verify.
- `lf repair --print-prompt` — print the generated prompt and exit.
- `lf repair --attempts <n>` — override `repair.max_attempts`.
- `lf repair --agent <name>` — pick an agent from the `agents:` map.
- `lf repair --from-latest-failed` — repair the most recent **failed** run even
  if a newer passing run exists.
- `lf repair --no-verify` — invoke the agent once without rerunning verify
  (cannot confirm a fix; for debugging the agent wiring).

If the latest failed evidence is **stale** (the working tree changed since that
run), repair still runs but prints a warning and notes it in the prompt.

#### Artifacts

Each attempt writes under the original failed run's directory:

```
.lf/runs/<original-failed-run>/repair/
  attempt-1/
    prompt.md          # the exact prompt sent to the agent
    agent.stdout.txt
    agent.stderr.txt
    result.json        # agent name/command/args/exit code
  attempt-2/ ...
  summary.md           # original run, failed commands, attempts, final result
```

Each verify rerun creates its own normal `.lf/runs/<timestamp>/` evidence.

#### Config

Repair is configured in `.lunarforge.yml`. The agent abstraction is small: a
name, an informational backend label, a command, and fixed args. LunarForge
writes the generated prompt to the command's **stdin**, which both
`claude --print` and `codex exec -` accept.

```yaml
repair:
  enabled: true
  max_attempts: 3
  verify_after_each_attempt: true
  max_log_chars: 20000        # truncate inlined logs; full logs stay on disk
  agent: claude_repair        # default agent (override with --agent)

agents:
  claude_repair:
    backend: claude_code
    command: claude
    args:
      - --print
      - --permission-mode
      - acceptEdits

  codex_repair:
    backend: codex
    command: codex
    args:
      - exec
      - --sandbox
      - workspace-write
      - "-"                   # read the prompt from stdin
```

**Claude Code** (researched against CLI `2.1.196`): `--print` enables
non-interactive mode and reads the prompt from stdin; `--permission-mode
acceptEdits` auto-applies file edits. Note the older `--max-turns` flag has been
**removed** from current Claude Code — the current spend guard is
`--max-budget-usd`. To restrict tools, use `--tools Read,Edit,Bash` (limits which
built-in tools exist), `--allowedTools` (auto-approve specific calls without
prompting, e.g. `--allowedTools "Bash(go build:*)"`), and `--disallowedTools`
(deny scoped calls, e.g. `--disallowedTools "Bash(git push:*)"`). These three are
distinct:

```
--tools           restrict which tools are available at all
--allowedTools    allow selected tool calls without prompting
--disallowedTools deny tools or scoped tool calls
```

**Codex** (`codex exec`): the safe default for repairs is
`--sandbox workspace-write` (edit files in the workspace, but not the wider
host). The trailing `-` makes `codex exec` read the prompt from stdin. **Do not**
use `--sandbox danger-full-access` for repairs.

Exact flags evolve — run `claude --help` / `codex exec --help` and edit `args`
directly to match your installed CLI.

#### Safety

This is **not** a sandbox. LunarForge controls the prompt and reruns
verification, but the repair agent still runs **on your machine with whatever
permissions you give it**. Prefer a conservative agent: for Claude, restrict
tools and deny pushes/destructive commands; for Codex, prefer
`--sandbox workspace-write`. Avoid `danger-full-access`, `bypassPermissions`, and
`--dangerously-skip-permissions` unless you deliberately opt in.

### `lf install-hooks`

Installs a **pre-push** hook (not pre-commit — pre-commit is too noisy for WIP
commits). The hook runs `lf status --require-fresh-passing`, so a push is blocked
unless there is **fresh, passing evidence** for the current code. The hook only
**reads** saved evidence; it does **not** re-run your tests, so it's fast.

The hook is safe about existing hooks:

- A previously LunarForge-managed hook is updated in place.
- An existing **foreign** `pre-push` hook is **backed up** (e.g.
  `pre-push.backup-20260630T142210`) before the new one is written, so nothing
  is silently destroyed.
- It honors `core.hooksPath` if you've configured one.
- It is a POSIX `sh` script and is made executable on Unix-like systems. On
  Windows, Git for Windows ships its own `sh`, so the hook runs there too.

#### Hooks are local — mirror them remotely later

A git pre-push hook is a **local** convenience and can be bypassed with
`git push --no-verify`. It is not a server-side guarantee. The intended end
state is to mirror the same `.lunarforge.yml` checks in **GitHub Actions** (or
another CI) so the remote enforces what the local hook encourages. That remote
mirror is on the [roadmap](#roadmap) and intentionally not part of this MVP.

---

## Try it on the fixture

A self-contained fixture under [`examples/fixture-basic/`](examples/fixture-basic/)
proves the whole loop end-to-end with **no Node, CMake, or Claude required**. Its
verify step just checks that `src/hello.txt` contains the expected text, and its
explain command is a fake local script.

```bash
cp -r examples/fixture-basic /tmp/lf-demo && cd /tmp/lf-demo
git init
git add .
git commit -m "fixture"

lf verify                          # ✅ contents passed → evidence saved
lf status                          # ✅ passed, ✅ fresh → ready to push
lf status --require-fresh-passing  # exits 0
lf explain                         # runs scripts/fake-explain.sh, saves explanation
```

Now change a tracked file and watch the evidence go stale:

```bash
echo "change" >> src/hello.txt
lf status                          # ⚠️ stale → not ready to push
lf status --require-fresh-passing  # exits non-zero
```

And see the pre-push gate in action:

```bash
lf install-hooks
git add -A && git commit -m "change"
git push        # blocked: evidence is stale for this commit
lf verify       # re-prove for the new commit
git push        # now allowed
```

### Repair fixture

A second fixture under
[`examples/fixture-repair-basic/`](examples/fixture-repair-basic/) demonstrates
`lf repair` end-to-end with **no Claude or Codex required**. It ships in a
**failing** state (`src/hello.txt` contains `broken`, but verify expects
`hello lunarforge`) and configures fake repair agents: `fake_success` (edits the
file so verify passes) and `fake_noop` (changes nothing).

```bash
cp -r examples/fixture-repair-basic /tmp/lf-repair-demo && cd /tmp/lf-repair-demo
git init
git add .
git commit -m "fixture"

lf verify                 # ❌ contents failed → failed evidence saved
lf repair --dry-run       # shows the plan + agent command, writes nothing
lf repair                 # fake agent fixes the file, verify reruns → ✅ passed
lf status --require-fresh-passing   # exits 0

# Exhaustion path with the no-op agent:
git checkout src/hello.txt && printf 'broken\n' > src/hello.txt
git commit -am break
lf verify
lf repair --agent fake_noop --attempts 2   # ❌ not repaired after 2 attempts
```

---

## Roadmap

The core is deliberately small. The code is structured (config / gitutil /
evidence / runner / explain / repair / hooks) so these can be added later
without a rewrite:

- Remote backup via GitHub Actions (the remote mirror of `lf verify`).
- Richer explain modes and model-per-step selection.
- Autonomous implementation from vague tasks (`lf repair` stays narrow — failed
  gates only — and does not do this).
- Integrations (issue trackers, multi-agent orchestration).
- Remote server / dashboard / multi-machine workers.

The principle stays the same: **local verification + evidence + explanation,
done well, with a narrow, opt-in repair of failed gates.**

---

## Project layout

```
cmd/lf/                 # CLI entrypoint and per-command files
internal/
  config/               # .lunarforge.yml loading + validation + starter template
  gitutil/              # repo checks, status/diff, deterministic diff hash
  evidence/             # evidence.json shape, read/write, latest pointer
  runner/               # runs verify commands, captures output, writes summary
  explain/              # builds the prompt, invokes the explain agent
  repair/               # builds the repair prompt, invokes the repair agent
  hooks/                # installs the pre-push hook
examples/
  node/.lunarforge.yml
  cpp/.lunarforge.yml
  scripts/verify.sh
  scripts/verify.ps1
  fixture-basic/            # self-contained end-to-end fixture (no external deps)
    .lunarforge.yml
    src/hello.txt
    scripts/verify.sh
    scripts/verify.ps1
    scripts/fake-explain.sh
  fixture-repair-basic/     # self-contained `lf repair` fixture (fake agents)
    .lunarforge.yml
    src/hello.txt           # ships "broken" so verify fails
    scripts/verify.sh
    scripts/verify.ps1
    scripts/fake-repair-success.sh
    scripts/fake-repair-noop.sh
```

## Development

```bash
go build ./...
go test ./...
gofmt -l .
```
