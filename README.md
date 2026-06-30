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
AGENTS.md / CLAUDE.md = reminders for agents.
scripts/verify.sh     = real repo ritual.
lf verify             = local evidence.
lf explain            = diff understanding.
pre-push hook         = local enforcement before pushing.
GitHub Actions        = remote backup later.
```

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

A small, maintainable CLI (`lf`) that does four things well:

1. **`lf verify`** — runs your configured commands and records evidence.
2. **`lf status`** — tells you if the latest evidence is fresh and passing.
3. **`lf explain`** — explains the current diff using git + the latest evidence.
4. **`lf install-hooks`** — installs a pre-push gate.

## What LunarForge is *not* (yet)

It is intentionally **not** an agent framework. It does **not** do autonomous
implementation, repair loops, multi-agent workflows, remote servers,
dashboards, GitHub Actions generation, or multi-machine routing. See
[Roadmap](#roadmap).

The MVP is exactly: **local verification + evidence + explanation.**

---

## The intended workflow

```
1. You manually drive Claude Code / Codex / another agent.
2. The agent edits files.
3. You run `lf verify`.
4. LunarForge runs your repo's required local lint/build/test command(s).
5. LunarForge saves evidence tied to the exact current git diff.
6. You run `lf explain`.
7. LunarForge explains what changed, using the current diff + latest evidence.
8. You review from a much better place.
```

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

## Quick start

```bash
cd your-repo
lf init                 # creates .lunarforge.yml and .lf/
# edit .lunarforge.yml, write scripts/verify.sh
lf verify               # run checks, save evidence
lf status               # is evidence fresh + passing?
lf explain              # explain the current diff
lf install-hooks        # block pushes without fresh passing evidence
```

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
first failure (use `lf verify --keep-going` to run them all).

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
5. Stops on the first failure by default (`--keep-going` to override).
6. Saves evidence under `.lf/runs/<timestamp>/` and updates `.lf/latest`.

```
LunarForge verify

✅ lint passed
✅ typecheck passed
✅ test passed
✅ build passed

Evidence saved:
.lf/runs/2026-06-30T14-22-10/evidence.json

Diff hash:
sha256:abc123...
```

On failure it prints `Not ready.`, still saves evidence, and exits non-zero.

#### The diff hash

Evidence is bound to the current change via a deterministic SHA-256 of:

```bash
git diff --binary           # tracked, unstaged changes
git diff --cached --binary  # staged changes
git status --porcelain      # which files are added/modified/untracked
```

If your **tracked or staged** working-tree changes change after `lf verify`,
the hash changes and the evidence becomes **stale**. (Note: this scope is by
design. Editing the *contents* of an untracked file only changes the hash once
the file is tracked/staged — untracked files register by name via
`git status --porcelain`.)

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

Loads the latest evidence, recomputes the current diff hash, and reports
whether the evidence is **fresh** (matches the current diff) and **passing**.

```
LunarForge status

Latest run:
  .lf/runs/2026-06-30T14-22-10

Result:
  ✅ passed

Evidence:
  ⚠️ stale — current diff changed after verification

Run:
  lf verify
```

`lf status --strict` exits non-zero unless the evidence is **fresh and
passing**. This is what the pre-push hook uses.

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

The generated prompt is **always** saved to
`.lf/runs/<run>/explain-prompt.md` first — so if the explain command is missing
or fails, you still have the prompt to run manually. Use `lf explain
--prompt-only` to build the prompt without invoking any agent.

### `lf install-hooks`

Installs a **pre-push** hook (not pre-commit — pre-commit is too noisy for WIP
commits). The hook runs `lf status --strict`, so a push is blocked unless there
is **fresh, passing evidence** for the current diff:

```
pre-push requires fresh passing evidence
```

Bypass once with `git push --no-verify`. The hook is safe about existing hooks:

- A previously LunarForge-managed hook is updated in place.
- An existing **foreign** `pre-push` hook is **backed up** (e.g.
  `pre-push.backup-20260630T142210`) before the new one is written, so nothing
  is silently destroyed.

It honors `core.hooksPath` if you've configured one.

---

## Roadmap

The MVP is deliberately small. The code is structured (config / gitutil /
evidence / runner / explain / hooks) so these can be added later without a
rewrite:

- Remote backup via GitHub Actions (the remote mirror of `lf verify`).
- Richer explain modes and model-per-step selection.
- Autonomous implementation / repair loops.
- Integrations (issue trackers, multi-agent orchestration).
- Remote server / dashboard / multi-machine workers.

None of these exist yet, and that's the point: **local verification + evidence
+ explanation, done well, first.**

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
examples/
  node/.lunarforge.yml
  cpp/.lunarforge.yml
  scripts/verify.sh
  scripts/verify.ps1
```

## Development

```bash
go build ./...
go test ./...
gofmt -l .
```
