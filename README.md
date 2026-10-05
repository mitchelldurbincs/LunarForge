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
GitHub Actions        = remote backup (authoritative for PRs)
```

The **local** loop (`lf verify` → `lf status` → pre-push hook) is fast and
convenient, but a local hook can be bypassed with `git push --no-verify`. The
**remote** loop (GitHub Actions running `lf ci`) re-runs the *same*
`.lunarforge.yml` verify commands on the PR, so branch protection can make the
gate authoritative — something a local CLI flag can't wave past.

LunarForge does **not** replace Claude Code, Codex, or manual driving. It is the
**local evidence layer that runs after an AI edits your code**: it records command outcomes and the tested subject. Commit mode checks a clean
HEAD in isolation; the hook refuses unsupported ref operations and missing,
failed, dirty, or stale commit evidence. Local records are not authenticated proof.

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
6. **`lf ci`** — runs the same verify commands in CI (the remote mirror).
7. **`lf gen-actions`** — generates a GitHub Actions workflow that runs `lf ci`.

## What LunarForge is *not* (yet)

It is intentionally **not** an agent framework. It does **not** do autonomous
implementation from vague tasks, `lf run "build feature"`, multi-agent
workflows, remote servers, dashboards, or multi-machine routing. `lf repair` is
deliberately narrow: it only reacts to a **failed** verification run and tries
to make that exact gate pass — it does not do open-ended feature work. See
[Roadmap](#roadmap).

The core is: **local verification + evidence + explanation, with an opt-in,
narrow repair of failed gates**, plus a thin **remote mirror** of the same
checks in GitHub Actions.

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
8. When a human pushes, the hook validates stdin refs and checks clean HEAD evidence.
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

lf loop                             # verify → repair if needed → explain when verified

# inspect and commit any repair changes first:
git diff
lf verify --commit HEAD             # clean candidate, isolated execution
lf status --commit HEAD --require-fresh-passing
# Human handoff: pushing is a separate human action, never an AI action.
```

`lf loop` runs the standard sequence in one command. The individual commands
(`lf verify`, `lf repair`, `lf explain`) are still there when you want finer
control — see [Local loop](#local-loop) for when to use each.

The hook supports one branch update whose outgoing object is HEAD. It requires
clean commit evidence. A loop may repair uncommitted files; inspect and commit
those changes, then run `lf verify --commit HEAD` before the human push stage.

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

Legacy diff-mode evidence describes HEAD plus local changes using a deterministic
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

**Legacy diff-mode limitations:**

A passing dirty worktree does not prove HEAD passed. Earlier versions could
accept that dirty evidence for a push; strict status now refuses it, and the
installed hook requires the separate clean commit mode. Ignored inputs and
external config changes remain outside the legacy diff hash.

- The *contents* of an **untracked** file are not hashed — an untracked file
  registers only by name via `git status --porcelain`. Track or stage a file to
  include its contents in the diff hash. Dirty evidence cannot satisfy strict status.
- The hash reflects HEAD plus uncommitted changes, not the full file tree. If
  you `lf verify` a dirty tree and then commit those exact changes, re-run
  `lf verify` so the evidence is tied to the new commit (committing changes the
  hash). The recommended loop — *commit, then verify* — avoids this.

#### Clean commit mode and external companion config

`lf verify --commit HEAD` refuses staged, unstaged, or untracked source changes.
It creates a disposable local clone, checks out HEAD without hooks, and runs the
configured commands there. Existing ignored files are not copied, eliminating
ambiguity from old build outputs. Submodules are currently refused. The original
checkout and its hooks are not changed. Commands must not refer back to mutable
files in the original checkout. This is input isolation, not an execution sandbox.

Records add full commit and tree IDs, repository identity, the effective config
and declared-input digest, OS/architecture, runner/Go/Git/shell identities,
declared tool versions, and start/end identity checks. Both the source checkout
and execution checkout are rechecked; a changed subject/config/platform fails the
run even when every command exited zero. The execution directory is removed on
completion. Untracked, non-ignored source outputs in the clone fail the end check.
Export artifacts to `LUNARFORGE_RUN_DIR` (set for each command).

The digest covers the whole config except reporting status, so CI, repair, agent,
and evidence-directory edits invalidate evidence too. Status and pre-push execute
configured `tool_versions` commands; rebuilding LF with a different Go version
also invalidates evidence.

Set `LUNARFORGE_CONFIG` to explicitly load a companion YAML file. An empty,
missing, or invalid explicit path fails without fallback. Otherwise discovery
stops at the repository boundary. Commands always start at the invocation's Git
root, including nested invocations and linked worktrees. External configs require
an absolute evidence directory whose parent is outside the repository; YAML does
not expand `~` or environment placeholders. Select a config for each invocation;
there is no automatic repository/profile mapping or YAML merge.

Example external file (replace absolute paths for your machine):

```yaml
version: 1
project:
  name: example
  repository_id: stable-repository-id
verify:
  profile: linux
  tree_reuse: false
  tool_versions:
    dotnet: dotnet --version
  # List absolute external scripts/config inputs here; their contents are hashed.
  inputs: []
  commands:
    - id: restore
      run: dotnet restore
    - id: build
      run: dotnet build -c Release --no-restore -warnaserror
    - id: test
      run: dotnet test -c Release --no-build --logger trx --results-directory "$LUNARFORGE_RUN_DIR/test-results"
evidence:
  dir: /home/me/.local/state/lunarforge/example/checkout-key/linux/runs
repair:
  enabled: false
status:
  contracts:
    - id: windows-restructure
      platform: windows
      status: pending
      reason: Windows platform not available on this runner
    - id: coverage-warnings
      status: enforced-remotely
      authority: ADO
      ado:
        definition_id: 111
```

```bash
LUNARFORGE_CONFIG="$HOME/.config/lunarforge/repos/example/linux.yml" lf verify --commit HEAD
LUNARFORGE_CONFIG="$HOME/.config/lunarforge/repos/example/linux.yml" lf status --json
```

Use a stable repository key, a distinct checkout key, and a platform/profile
namespace. `latest` and `loops/` are siblings of `runs/`. All LF artifacts stay
there; build outputs stay in the disposable checkout unless commands explicitly
choose an external cache or artifact directory. Runtime state belongs under the
user state directory; configuration belongs under the user config directory.
On Windows choose absolute paths under the corresponding user directories.

Completed evidence records are immutable through LF. Random run-ID suffixes and
exclusive directory creation prevent collisions; evidence and latest files are
published using sibling temporary files and rename. Commit status scans completed
ledger records, so a newer applicable failure blocks an older success even if
concurrent completion order moved `latest` backward. Running/incomplete attempts
are not completed evidence. Older schema-v1 diff records remain readable.
New evidence files use mode 0600 and state directories 0700; CI artifact readers
need access as the owning user.

History-only rewrites are stale by default. Opt into `verify.tree_reuse: true`
only for history-independent gates. A matching repository/tree/contract/platform
can then reuse the newest applicable execution: status sets `reused_from` and
reports the original execution SHA and timestamps without rewriting the ledger.
SourceLink, version stamping, Git history queries, unpinned dependencies, and
undeclared external inputs can invalidate tree-only assumptions. Record relevant
tools and external inputs; LF does not infer a complete dependency/environment
closure. Start/end checks cannot detect a transient edit reverted during a run.
Evidence is local operational accountability, not tamper-resistant attestation.

`lf status --json` preserves existing fields and adds current/evidence identities,
resolved paths, execution timestamps, `reused_from`, and `contracts`. The local
row reports freshness and result; external rows can only be `pending` or
`enforced-remotely`, never a fabricated local pass. Optional `ado` observations
carry `build_id`, `definition_id`, `source_commit`, `target_commit`, `merge_commit`,
`result`, and `observed_at` independently. LF performs no ADO reads or writes;
the orchestrator obtains and refreshes those observations. Reporting-only changes
do not change the execution digest. `ready` is local gate readiness;
`all_contracts_satisfied` remains false when external requirements are present.
Neither field grants merge approval. Pushing remains a human stage.

The pre-push contract supports one branch update with a local object equal to
HEAD. It fails closed with these messages for unsupported operations:

- `pre-push: local object is not HEAD`
- `pre-push: multiple ref updates are not supported`
- `pre-push: deletion updates are not supported`
- `pre-push: tag updates are not supported`
- `pre-push: only branch updates are supported`
- `pre-push: no ref updates supplied`

Malformed fields/object IDs also fail. Branch creation is supported. A GUI Git
client may not inherit your shell environment: manually install a repo-scoped
wrapper that exports the exact companion config before invoking `lf pre-push`.
Inspect `core.hooksPath` and preserve existing hook behavior; a backed-up foreign
hook is not automatically composed. Hooks remain bypassable and are not server
policy. These changes provide data for a future dashboard, not a dashboard UI.

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
  "git": { "branch": "main", "head": "<full-commit-SHA>", "status_porcelain": "..." },
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

`lf status --require-fresh-passing` makes the exit
code the source of truth. It exits:

- **`0`** only when evidence **exists**, **passed**, is **fresh** under its mode,
  and both the current and tested subjects are clean. With `--commit HEAD`,
  legacy records cannot qualify; full commit/tree/contract/platform identity is checked.
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

### Local loop

`lf loop` chains the existing local commands into one repeatable sequence:

```
lf loop = verify → repair if needed → explain when verified
```

It is the one-command version of the manual ritual. Daily usage:

```bash
# manually drive Claude Code / Codex first; the agent edits files
git add -A && git commit -m "..."

lf loop

# if successful:
git diff
git push
```

What it does, exactly:

```
1. Run lf verify.
2. If verify passes:
   - run lf explain.
   - result: ready for review.
3. If verify fails:
   - run lf repair (which reverifies after each attempt).
   - re-check the latest evidence:
     - if it is now fresh and passing: run lf explain → repaired and ready for review.
     - if it is still failing: skip explain → blocked.
```

`lf loop` is **not autonomous feature work**:

```
It does not decide what to build.
It does not start from a task description.
It only checks and repairs the current working tree and .lunarforge.yml.
```

The agent still does **not** get to declare success. The loop trusts only
LunarForge evidence: after repair it re-reads the latest evidence and runs
`lf explain` only when that evidence is fresh and passing. A successful repair
can leave dirty files: strict status refuses that subject even when the loop
passes. Commit the inspected repair and verify the resulting clean HEAD.

**Flags:**

- `lf loop --no-repair` — run verify; if it fails, stop (strict check, no AI repair).
- `lf loop --no-explain` — run verify and repair if needed, but skip the explanation.
- `lf loop --repair-attempts <n>` — override `repair.max_attempts` for this loop.
- `lf loop --continue-on-failure` — forwarded to verify: run all commands even after one fails.
- `lf loop --dry-run` — print the steps that would run without running verify, repair, or explain.

**Artifacts.** Each loop writes a small summary that links to (does not duplicate)
the underlying evidence and repair artifacts:

```
.lf/loops/<timestamp>/
  summary.md     # human-readable: verify/repair/explain outcomes + final result
  loop.json      # machine-readable: timings, attempts, evidence + explanation paths
```

The verify and repair steps still write their normal `.lf/runs/<timestamp>/`
evidence; the loop summary just points at the final one.

**Lower-control alternatives.** Reach for the individual commands when you want
finer control:

```bash
lf verify     # only want proof the checks pass
lf repair     # a gate already failed and you want the agent to fix it
lf explain    # want a review summary of the current diff
lf loop       # want the standard local sequence in one command
```

### `lf install-hooks`

Installs a **pre-push** hook (not pre-commit — pre-commit is too noisy for WIP
commits). The hook runs `lf pre-push`, consuming Git's stdin refs, then checks
`lf status --commit HEAD --require-fresh-passing`. The hook only
**reads** saved evidence; it does **not** re-run your tests, so it's fast.

Review existing hooks before installation:

- A previously LunarForge-managed hook is updated in place.
- An existing **foreign** `pre-push` hook is **backed up** (e.g.
  `pre-push.backup-20260630T142210`) before the new one is written, so nothing
  is silently destroyed.
- It honors `core.hooksPath` if you've configured one.
- It is a POSIX `sh` script and is made executable on Unix-like systems. On
  Windows, Git for Windows ships its own `sh`, so the hook runs there too.

#### Hooks are local — GitHub Actions is the remote mirror

A git pre-push hook is a **local** convenience and can be bypassed with
`git push --no-verify`. It is not a server-side guarantee. The remote mirror —
**GitHub Actions** running the same `.lunarforge.yml` checks — is what makes the
gate authoritative. See [GitHub Actions mirror](#github-actions-mirror).

---

## GitHub Actions mirror

The local hook is **bypassable-but-useful**; GitHub Actions is **authoritative**.
The remote workflow runs `lf ci`, which executes the *same* `verify.commands`
from `.lunarforge.yml` — so there's a single source of truth and no drift
between local and remote.

```
AGENTS.md / CLAUDE.md = reminders
scripts/verify.sh     = repo ritual
lf verify             = local proof
lf status             = fresh/stale proof check
pre-push hook         = local enforcement (bypassable with --no-verify)
GitHub Actions        = remote backup (authoritative for PRs/branches)
```

The mental model for the three verify-shaped commands:

```
lf verify = local proof for the current working tree
lf status = is the latest local proof still valid?
lf ci     = remote proof for the current CI checkout
```

### Do not duplicate your commands

The workflow **delegates** to LunarForge instead of re-listing your build/test
commands:

```yaml
# ✅ Good — one source of truth
- run: ./lf ci
```

```yaml
# ❌ Bad — drifts from .lunarforge.yml
- run: npm run lint
- run: npm test
- run: npm run build
```

### `lf ci`

`lf ci` is the CI-friendly verification command. It loads `.lunarforge.yml`,
confirms it's in a git repo, runs the configured `verify.commands`, and saves
evidence under `.lf/runs/<timestamp>/` (so CI can upload it as an artifact).
It exits `0` when all required commands pass and non-zero when any fails.

Unlike `lf verify`, **`lf ci` does not care about pre-existing fresh local
evidence** — in CI the current checkout *is* the source of truth, so it just
runs the commands. When `GITHUB_ACTIONS=true`, it also emits an `::error::`
annotation on failure and writes a short result table to the job summary.

```
LunarForge CI

✅ lint passed       1.2s
✅ test passed       5.4s

Result:
✅ CI verification passed

Evidence:
.lf/runs/2026-06-30T14-22-10/evidence.json
```

### `lf gen-actions`

Generates `.github/workflows/lunarforge.yml`:

```bash
lf gen-actions                                       # default path, auto-detected mode
lf gen-actions --install-mode source                 # build lf from ./cmd/lf
lf gen-actions --install-mode go-install             # go install lf (consumer repos)
lf gen-actions --install-mode go-install --install-ref v0.1.0
lf gen-actions --output .github/workflows/x.yml      # custom path
lf gen-actions --force                               # overwrite an existing file
```

It will **not** overwrite an existing workflow unless `--force` is passed, and
prints the path plus next steps. The generated workflow:

- runs on pull requests and pushes to `main`,
- uses `concurrency` to cancel superseded runs,
- uses minimal `permissions: contents: read`,
- obtains `lf` according to the **install mode** (see below) and runs `lf ci`,
- uploads `.lf/runs/**` as an artifact (`if: always()`).

#### Install modes

`lf gen-actions` needs to know how the workflow should get the `lf` binary.
There are three modes:

| Mode | For | How the workflow gets `lf` | Runs |
|---|---|---|---|
| `source` | the **LunarForge repo itself** | `go build -o lf ./cmd/lf` | `./lf ci` |
| `go-install` | a **normal repo using LunarForge** | `go install <module>/cmd/lf@<ref>` | `lf ci` |
| `custom` | release binaries / curl | your `install_commands` | `lf ci` |

When `--install-mode` is omitted, the mode comes from
`ci.github_actions.install.mode` in `.lunarforge.yml`, or is **auto-detected**:

- if `./cmd/lf` **exists**, default to **source** mode (you're in LunarForge);
- otherwise default to **go-install** mode (a consumer repo).

For `go-install`, the module path is read from your `go.mod`
(`<module>/cmd/lf`), falling back to
`github.com/mitchelldurbincs/lunarforge/cmd/lf`. Override it with
`--install-module`, and pin the version with `--install-ref`
(`latest` by default).

`custom` mode is configured in `.lunarforge.yml` and runs explicit install
steps before `lf ci`:

```yaml
ci:
  github_actions:
    install:
      mode: custom
      install_commands:
        - curl -L https://example.com/lf -o lf
        - chmod +x lf
        - sudo mv lf /usr/local/bin/lf
```

### Recommended setup

```bash
lf gen-actions
git add .github/workflows/lunarforge.yml
git commit -m "add LunarForge CI"
git push
```

Then, in **GitHub → Settings → Branches → Branch protection rules**, require the
**LunarForge / Verify** check to pass before merging. Local hooks can be skipped
with `git push --no-verify`; a required GitHub check **cannot** be bypassed by a
local CLI flag.

### Important limitation: setup is your job

The generated workflow is only a **remote mirror of your verify commands**. It
does **not** install your project's dependencies or toolchain:

- Node repos still need Node set up + `npm ci`.
- Rust repos still need the Rust toolchain.
- C++ repos may need CMake / a compiler.
- Windows desktop repos may need `runs-on: windows-latest`.

You have two options:

1. **Edit the generated workflow** and add the setup steps you need (see the
   ready-to-copy examples in [`examples/github-actions/`](examples/github-actions/)).
2. **Use `ci.setup_commands`** in `.lunarforge.yml` to have a simple "Project
   setup" step generated for you:

   ```yaml
   ci:
     setup_commands:
       - npm ci
   ```

Optional CI config (all fields are optional; defaults are shown):

```yaml
ci:
  github_actions:
    workflow_name: LunarForge   # name: of the workflow
    runs_on: ubuntu-latest      # runner
    timeout_minutes: 30         # job timeout
    upload_artifacts: true      # upload .lf/runs/** as an artifact
    install:
      mode: go-install          # source | go-install | custom (default: auto-detect)
      module: github.com/mitchelldurbincs/lunarforge/cmd/lf  # go-install target
      ref: latest               # go-install version/ref
      install_commands: []      # custom-mode install steps
  setup_commands: []            # commands run before `lf ci`
```

Example workflows for common stacks:

- [`lunarforge-source.yml`](examples/github-actions/lunarforge-source.yml) — **source mode** (developing LunarForge itself).
- [`lunarforge-go-install.yml`](examples/github-actions/lunarforge-go-install.yml) — **go-install mode** (a normal repo using LunarForge).
- [`lunarforge-basic.yml`](examples/github-actions/lunarforge-basic.yml) — self-contained source-mode workflow.
- [`lunarforge-node.yml`](examples/github-actions/lunarforge-node.yml) — Node consumer: setup-node + `npm ci`.
- [`lunarforge-windows.yml`](examples/github-actions/lunarforge-windows.yml) — `windows-latest` for C++/desktop.

---

## Using generated workflows in consumer repos

LunarForge enforces the same gate at three layers. Local is fast and
bypassable; remote is authoritative; workflow generation is how you set the
remote layer up:

```
local:
  lf verify                          # prove the working tree passes
  lf status --require-fresh-passing  # is the latest proof still valid?
  pre-push hook                      # local enforcement (bypassable with --no-verify)

remote:
  lf ci inside GitHub Actions        # authoritative re-run of the same checks

workflow generation:
  lf gen-actions --install-mode source       # for LunarForge itself
  lf gen-actions --install-mode go-install    # for normal repos
```

The distinction that makes generation actually usable in a real project is the
**install mode**. The LunarForge repo contains `./cmd/lf` and can build `lf`
from source. A normal project that *uses* LunarForge does **not** — so the
generated workflow must install `lf` instead of building it.

```bash
# In the LunarForge repo (source is auto-detected because ./cmd/lf exists):
lf gen-actions --install-mode source

# In a normal project repo (go-install is auto-detected because there is no ./cmd/lf):
lf gen-actions --install-mode go-install --install-ref latest
```

The go-install workflow runs `lf ci` (the binary is on `PATH`), never
`./lf ci`, and never references `./cmd/lf` — so it does not assume the consumer
repo contains LunarForge source. A self-contained example of a consumer repo
lives in
[`examples/fixture-consumer-basic/`](examples/fixture-consumer-basic/): it has a
`.lunarforge.yml`, a `verify.sh`/`verify.ps1`, and a `src/hello.txt`, but **no
`cmd/lf`**.

### Limitation: `@latest` needs an installable module

`go install <module>/cmd/lf@latest` only works once the LunarForge module is
actually fetchable by `go install` at that ref — i.e. it's a public module (or
reachable via your `GOPRIVATE`/proxy config) and the ref exists. Until tagged
releases exist you have a few honest options:

- pin a **branch or commit** instead of a tag, e.g.
  `lf gen-actions --install-mode go-install --install-ref main` (Go module
  pseudo-versions accept a branch name or commit SHA);
- use **`custom` mode** to download a prebuilt binary in CI;
- or, while iterating, vendor LunarForge into the consumer repo and use
  `source` mode.

When real releases exist, switch back to `--install-ref v0.1.0` (or `latest`)
for a clean, version-pinned install.

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
lf verify --commit HEAD  # isolated verification of the new commit
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
lf status --require-fresh-passing   # non-zero: repair is still uncommitted

# Exhaustion path with the no-op agent:
git checkout src/hello.txt && printf 'broken\n' > src/hello.txt
git commit -am break
lf verify
lf repair --agent fake_noop --attempts 2   # ❌ not repaired after 2 attempts
```

### Loop fixture

A third fixture under
[`examples/fixture-loop-basic/`](examples/fixture-loop-basic/) demonstrates
`lf loop` end-to-end with **no Claude or Codex required**. Unlike the repair
fixture it ships **passing**, so the immediate-success path works out of the box;
break `src/hello.txt` to exercise repair and the blocked path.

```bash
# 1) Immediate success: verify passes, repair skipped, explain runs.
cp -r examples/fixture-loop-basic /tmp/lf-loop-pass && cd /tmp/lf-loop-pass
git init && git add . && git commit -m "fixture"
lf loop                              # ✅ ready for review
lf status --require-fresh-passing    # exits 0

# 2) Repair success: break the file, let the fake agent fix it.
cp -r examples/fixture-loop-basic /tmp/lf-loop-repair && cd /tmp/lf-loop-repair
git init && git add . && git commit -m "fixture"
echo broken > src/hello.txt
lf loop                              # ❌ verify → repair → ✅ repaired and ready for review
lf status --require-fresh-passing    # exits 0

# 3) Blocked: switch to the no-op agent so repair can't fix it.
cp -r examples/fixture-loop-basic /tmp/lf-loop-block && cd /tmp/lf-loop-block
sed -i 's/agent: fake_success/agent: fake_noop/' .lunarforge.yml
git init && git add . && git commit -m "fixture"
echo broken > src/hello.txt
lf loop --repair-attempts 2          # ❌ blocked, explain skipped
lf status --require-fresh-passing    # exits non-zero
```

---

## Roadmap

The core is deliberately small. The code is structured (config / gitutil /
evidence / runner / explain / repair / hooks / actions) so these can be added
later without a rewrite:

- Richer explain modes and model-per-step selection.
- Editable workflows beyond the fixed `lf loop` sequence (`lf loop` stays a
  single, non-autonomous chain of the existing local commands and does not do
  this yet).
- Autonomous implementation from vague tasks (`lf repair` and `lf loop` stay
  narrow — failed gates and the current working tree only — and do not do this).
- Integrations (issue trackers, multi-agent orchestration).
- Remote server / dashboard / multi-machine workers.

The principle stays the same: **local verification + evidence + explanation,
done well, with a narrow, opt-in repair of failed gates, a single boring loop
that chains them, and a thin remote mirror of the same checks.**

---

## Project layout

```
.lunarforge.yml         # LunarForge's own config — this repo gates itself
scripts/verify.sh       # the repo's real ritual: gofmt + vet + build + test
scripts/verify.ps1      # Windows twin of the above
.github/workflows/ci.yml # remote backup: runs the same ritual on Linux + Windows
cmd/lf/                 # CLI entrypoint and per-command files
internal/
  config/               # .lunarforge.yml loading + validation + starter template
  gitutil/              # repo checks, status/diff, deterministic diff hash
  evidence/             # evidence.json shape, read/write, latest pointer
  runner/               # runs verify commands, captures output, writes summary
  explain/              # builds the prompt, invokes the explain agent
  repair/               # builds the repair prompt, invokes the repair agent
  hooks/                # installs the pre-push hook
  actions/              # generates the GitHub Actions workflow (lf gen-actions)
cmd/lf/cmd_loop.go      # `lf loop`: composes verify → repair → explain
cmd/lf/loop_summary.go  # loop summary artifact (.lf/loops/<ts>/)
examples/
  node/.lunarforge.yml
  cpp/.lunarforge.yml
  scripts/verify.sh
  scripts/verify.ps1
  github-actions/           # ready-to-copy CI workflows (basic / node / windows)
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
  fixture-loop-basic/       # self-contained `lf loop` fixture (fake agents)
    .lunarforge.yml
    src/hello.txt           # ships PASSING; break it to exercise repair/blocked
    scripts/verify.sh
    scripts/verify.ps1
    scripts/fake-repair-success.sh
    scripts/fake-repair-noop.sh
    scripts/fake-explain.sh
```

## Development

LunarForge gates itself with LunarForge. The repo ships its own
`.lunarforge.yml` and `scripts/verify.sh`, so the ritual you run locally is the
ritual CI runs:

```bash
./scripts/verify.sh      # gofmt + go vet + go build + go test
```

Or drive it through the tool itself, once `lf` is on your `PATH`:

```bash
go build -o lf ./cmd/lf
./lf verify              # runs scripts/verify.sh, records evidence in .lf/runs/
./lf status              # fresh + passing?
./lf install-hooks       # gate your own pushes on it
```

`.github/workflows/ci.yml` runs the same script on Linux and its PowerShell twin
(`scripts/verify.ps1`) on Windows, so the cross-platform claim is checked rather
than asserted. The remote workflow is the backup; the local gate is the point.
