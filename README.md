# LunarForge (`lf`)

LunarForge is a deterministic verification gate for repositories changed by
people or coding agents.

```text
Hermes / Claude / Codex
        edits code
            ↓
      lf verify --json
            ↓
 deterministic checks + snapshot-bound evidence
            ↓
 PASS / FAIL / STALE / BLOCKED / ERROR
```

It does not choose work, edit code, invoke agents, or decide whether to retry.
The caller owns orchestration. LunarForge defines what “good” means, runs those
checks, and reports what it proved.

## Why use it?

A repository normally scatters its required checks across docs, agent prompts,
and CI YAML. LunarForge gives every caller one command and one result contract:

```bash
lf verify --json
```

The result is tied to the exact Git state: HEAD, staged and unstaged changes,
and the paths, modes, and contents of untracked files. A later edit makes old
evidence stale. LunarForge also detects a check that changes the repository
while verification is running.

## Install

LunarForge requires Go 1.24 or later.

```bash
go install github.com/mitchelldurbincs/lunarforge/cmd/lf@latest
lf version
```

To build this repository instead:

```bash
go build -o lf ./cmd/lf
```

## Configure a repository

Run `lf init`, then make `.lunarforge.yml` describe the checks that must pass:

```yaml
version: 1

verify:
  commands:
    - id: lint
      run: npm run lint
    - id: test
      run: npm test
      timeout_seconds: 900
    - id: build
      run: npm run build
```

That is the entire configuration schema. Commands run in order from the
repository root. They stop after the first failure unless
`--continue-on-failure` is used. Each command defaults to a 30-minute timeout.

Command IDs may contain letters, numbers, `.`, `_`, and `-`. They become log
filenames, so path-like IDs are rejected. Unknown configuration fields are
errors instead of being silently ignored.

For a project with a single canonical check script, the starter is enough:

```yaml
version: 1

verify:
  commands:
    - id: verify
      run: ./scripts/verify.sh
```

Commit `.lunarforge.yml`, the verification script, and the generated
`.lf/.gitignore`. Evidence under `.lf/` remains local.

## Commands

```text
lf init            create the minimal repository policy
lf verify          run checks and write evidence
lf status          evaluate the latest evidence against the current snapshot
lf install-hooks   install the optional pre-push gate
lf gen-actions     generate the canonical GitHub Actions workflow
lf ci              compatibility alias for lf verify
```

### Verify

```bash
lf verify
lf verify --json --quiet
lf verify --continue-on-failure
```

Check stdout and stderr are saved separately under `.lf/runs/<run>/checks/`.
Without `--quiet`, they are also streamed to the terminal. In JSON mode the
streams go to stderr, leaving stdout as one JSON document.

Every configured check receives a record. Checks not run after an earlier
problem have state `skipped` and a reason identifying the prior failure or
block.

### Status

```bash
lf status
lf status --json
lf status --require-fresh-passing
```

Plain `lf status` is informational. `--json` and
`--require-fresh-passing` return a nonzero exit code when the result is not
ready, making them suitable for programs and hooks.

`status` never runs checks. It validates and compares saved evidence. Missing
evidence and corrupt evidence are distinct results.

### Pre-push gate

```bash
lf install-hooks
```

The hook only accepts fresh passing evidence for the final, clean, checked-out
HEAD. The safe order is therefore:

```bash
git add -A
git commit -m "make the change"
lf verify
git push
```

The hook is fast because it calls `lf status --require-fresh-passing`; it does
not rerun checks. Like any local hook it can be bypassed with
`git push --no-verify`. Protect the remote branch with CI when enforcement must
be authoritative.

### GitHub Actions

```bash
lf gen-actions --install-ref latest
```

This writes `.github/workflows/lunarforge.yml` without overwriting an existing
file unless `--force` is supplied. The workflow installs LunarForge, runs
`lf verify --json`, and uploads `.lf/runs/**` even on failure.

Pin `--install-ref` to a release tag for reproducible CI. Project-specific
language setup can be added to the generated workflow; the actual lint, test,
and build policy should remain in `.lunarforge.yml`.

## Programmatic contract

`lf verify --json` and `lf status --json` emit schema version 1. A shortened
successful result looks like this:

```json
{
  "schema_version": 1,
  "command": "verify",
  "state": "pass",
  "reason": "checks_passed",
  "repository": {
    "root": "/repo",
    "branch": "main",
    "head": "abc1234",
    "dirty": false,
    "fingerprint": "sha256:..."
  },
  "run": {
    "id": "2026-08-30T12-00-00.000000000Z-a1b2c3d4",
    "evidence_path": ".lf/runs/.../evidence.json",
    "started_at": "2026-08-30T12:00:00Z",
    "finished_at": "2026-08-30T12:00:01Z",
    "state": "pass",
    "reason": "checks_passed",
    "branch": "main",
    "head": "abc1234",
    "dirty": false,
    "fingerprint": "sha256:...",
    "final_fingerprint": "sha256:..."
  },
  "checks": [
    {
      "id": "test",
      "run": "go test ./...",
      "state": "pass",
      "reason": "checks_passed",
      "exit_code": 0,
      "duration_ms": 814,
      "timeout_ms": 1800000,
      "stdout_path": ".lf/runs/.../checks/test.stdout.txt",
      "stderr_path": ".lf/runs/.../checks/test.stderr.txt"
    }
  ]
}
```

Failure results include the last 4,000 bytes of each check stream as
`stdout_tail` and `stderr_tail`, plus full log paths. This is normally enough
for an orchestrator to give a coding agent useful feedback without parsing
terminal prose.

Top-level states and process exit codes are stable within schema version 1:

| State | Exit | Meaning |
|---|---:|---|
| `pass` | 0 | All checks passed for one unchanged snapshot. |
| `fail` | 1 | A check ran and failed. |
| `stale` | 1 | The current snapshot differs from the verified snapshot. |
| `blocked` | 2 | Verification could not run as configured. |
| `error` | 3 | LunarForge or saved evidence failed internally. |

Stable reason codes include:

| Reason | Meaning |
|---|---|
| `checks_passed` | Every required check passed. |
| `check_failed` | A required check returned nonzero. |
| `snapshot_changed` | Source changed during or after verification. |
| `no_evidence` | No previous run exists. |
| `config_invalid` | `.lunarforge.yml` is missing or invalid. |
| `tool_unavailable` | A configured executable could not be invoked. |
| `timed_out` | A check exceeded `timeout_seconds`. |
| `evidence_corrupt` | The latest saved record cannot be trusted. |
| `internal_error` | Git, filesystem, or LunarForge itself failed. |

Consumers should branch on `state` and `reason`, not message text. `checks[]`
is the repair input; files under `.lf/` are durable detail and audit evidence.

## Hermes workflow

The intended integration is a normal subprocess loop:

1. Hermes chooses a task and asks a coding agent to make the change.
2. The agent returns control; Hermes commits the candidate change if clean-HEAD
   gating is desired.
3. Hermes runs `lf verify --json --quiet` in the repository.
4. On `pass`, Hermes may summarize, push, or open a PR.
5. On `fail`, Hermes sends only the failing check, reason, command, error, and
   stream tails back to the coding agent, then verifies the next candidate.
6. On `stale`, Hermes stops treating the old run as proof and verifies the
   current snapshot.
7. On `blocked`, Hermes fixes environment/configuration or asks for help; it
   should not ask an agent to “repair the tests.”
8. On `error`, Hermes reports an infrastructure problem and preserves the run
   directory for diagnosis.

Only LunarForge's result decides whether the candidate is verified. Retry and
abandonment policy stays in Hermes.

Shell sketch:

```bash
result_file=$(mktemp)
if lf verify --json --quiet >"$result_file"; then
  # state is pass
  hermes_open_pr "$result_file"
else
  # inspect state/reason/checks and choose retry, remediation, or escalation
  hermes_handle_verification "$result_file"
fi
```

## Evidence layout

```text
.lf/
  latest
  runs/
    <timestamp-and-random-id>/
      evidence.json
      summary.md
      checks/
        <id>.stdout.txt
        <id>.stderr.txt
```

Evidence and the `latest` pointer are written with temporary-file-and-rename
replacement. Run IDs combine nanosecond timestamps with randomness so rapid or
concurrent runs do not overwrite each other.

## Development

LunarForge dogfoods one canonical script:

```bash
./scripts/verify.sh
```

It checks formatting, vet, build, and tests. The root GitHub workflow builds
the branch's `lf` binary and executes `./lf verify --json`, then uploads the
evidence.

Before publishing a release, run the canonical script, merge through CI, create
an annotated semantic-version tag, and confirm a clean consumer repository can
run both `go install ...@<tag>` and `lf verify --json`.

## Scope

LunarForge intentionally does not contain agent selection, repair prompts,
retry loops, issue-tracker integration, remote workers, dashboards, servers,
or a plugin system. Those concerns belong to the orchestrator. The useful core
is one repository policy, deterministic execution, trustworthy evidence, and a
small versioned JSON interface.
