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

A small, maintainable CLI (`lf`) that does these things well:

1. **`lf verify`** — runs your configured commands and records evidence.
2. **`lf status`** — tells you if the latest evidence is fresh and passing.
3. **`lf explain`** — explains the current diff using git + the latest evidence.
4. **`lf install-hooks`** — installs a pre-push gate.
5. **`lf ci`** — runs the same verify commands in CI (the remote mirror).
6. **`lf gen-actions`** — generates a GitHub Actions workflow that runs `lf ci`.

## What LunarForge is *not* (yet)

It is intentionally **not** an agent framework. It does **not** do autonomous
implementation, repair loops, multi-agent workflows, remote servers,
dashboards, or multi-machine routing. See [Roadmap](#roadmap).

The core is: **local verification + evidence + explanation**, plus a thin
**remote mirror** of the same checks in GitHub Actions.

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
lf gen-actions                                  # default path
lf gen-actions --output .github/workflows/x.yml # custom path
lf gen-actions --force                          # overwrite an existing file
```

It will **not** overwrite an existing workflow unless `--force` is passed, and
prints the path plus next steps. The generated workflow:

- runs on pull requests and pushes to `main`,
- uses `concurrency` to cancel superseded runs,
- uses minimal `permissions: contents: read`,
- checks out the repo, builds `lf` from `./cmd/lf`, runs `./lf ci`,
- uploads `.lf/runs/**` as an artifact (`if: always()`).

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
  setup_commands: []            # commands run before `lf ci`
```

Example workflows for common stacks:

- [`lunarforge-basic.yml`](examples/github-actions/lunarforge-basic.yml) — self-contained (matches `lf gen-actions`).
- [`lunarforge-node.yml`](examples/github-actions/lunarforge-node.yml) — Node setup + `npm ci`.
- [`lunarforge-windows.yml`](examples/github-actions/lunarforge-windows.yml) — `windows-latest` for C++/desktop.

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

---

## Roadmap

The MVP is deliberately small. The code is structured (config / gitutil /
evidence / runner / explain / hooks / actions) so these can be added later
without a rewrite:

- Richer explain modes and model-per-step selection.
- Autonomous implementation / repair loops.
- Integrations (issue trackers, multi-agent orchestration).
- Remote server / dashboard / multi-machine workers.

None of these exist yet, and that's the point: **local verification + evidence
+ explanation + a thin remote mirror, done well, first.**

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
  hooks/                # installs the pre-push hook
  actions/              # generates the GitHub Actions workflow (lf gen-actions)
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
```

## Development

```bash
go build ./...
go test ./...
gofmt -l .
```
