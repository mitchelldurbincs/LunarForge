# Integrating with LunarForge

Run LunarForge as a subprocess in the repository. Use `lf verify --json --quiet`
to execute checks, and `lf status --json` to inspect saved evidence without
executing checks. `lf ci --json` is a verification alias whose `command` is `ci`.

The [result schema](../schemas/result-v1.schema.json) describes the CLI's stdout
envelope. The [configuration schema](../schemas/config-v1.schema.json) describes
the JSON-compatible value of `.lunarforge.yml`; it can also support YAML editor
validation. Both use JSON Schema draft 2020-12. The CLI additionally validates
unique check IDs and rejects multiple YAML documents.

## Read stdout even when the process exits nonzero

For a recognized command with valid flags and `--json`, stdout is one JSON
document. Check output streams to stderr unless `--quiet` is set; complete check
logs are saved either way. Keep stdout and stderr separate. Invalid CLI usage,
process termination, or failure to launch the binary may produce no envelope.

| State | Exit code | Consumer action |
|---|---:|---|
| `pass` | 0 | Accept the verified snapshot, subject to the consumer's own policy. |
| `fail` | 1 | Inspect failed checks and their output. |
| `stale` | 1 | Verify the current snapshot again. |
| `blocked` | 2 | Resolve missing evidence, configuration, tools, or time limits. |
| `error` | 3 | Diagnose LunarForge, Git, filesystem, or saved-evidence problems. |

A nonzero exit is often a valid result. Decode the envelope before deciding
whether the invocation itself failed. Validate `schema_version`, the expected
`command`, and the state/exit combination. Unsupported versions, invalid JSON,
or inconsistent results must never satisfy a verification gate. Unknown reason
codes should retain the containing state's meaning and be surfaced for diagnosis.

## Required and conditional fields

Every envelope includes `schema_version`, `command`, `state`, `reason`, and
`repository`. `repository.dirty` is always a JSON boolean, including `false`.
When source inspection cannot complete, repository identity fields may be absent;
the boolean alone is not proof of a clean repository.

| Outcome | `run` and `checks` | Top-level `error` |
|---|---|---|
| Passing, failing, or stale evidence | Present | Normally absent |
| A check is blocked, for example by timeout | Present | Normally absent; inspect the check's `error` |
| `blocked / no_evidence` | Absent | Absent |
| Invalid configuration or corrupt evidence | Absent | Present |
| An internal error before a usable result | Absent | Present |

Branch on the envelope state and reason **before requiring a run or check list**.
For example, `blocked / no_evidence` is a supported response on a new repository,
not an invalid envelope or a missing binary.

When a run exists, every configured check has a record, in configuration order.
A check can be `pass`, `fail`, `blocked`, or `skipped`. Skipped checks have exit
code `-1`, duration zero, and no log paths. An exit code of `-1` can also mean an
invocation did not start or was terminated; use the check's state and reason.
The CLI process exit code is separate from individual check exit codes.

Failed and blocked checks expose `stdout_tail` and `stderr_tail` when those logs
have content; empty tails are omitted. Each tail is
the last 4,000 bytes decoded with invalid UTF-8 replaced. Full log paths are
relative to `repository.root`. Treat messages and logs as display text, not
stable machine identifiers.

## Current state versus the saved run

`repository` describes the current observation. `run` describes the historical
verification. In particular, `state: stale` can accompany `run.state: pass`:
the old checks succeeded, but no longer verify the current tree. Use the
top-level state for current readiness. Check rows likewise describe the old run.

New snapshots record full Git object IDs in `repository.head`, `run.head`, and
the stored evidence's `git.head`. Compare them with `git rev-parse HEAD`, not
with a fixed-length prefix. An unborn repository uses `(none)`; it cannot satisfy
a consumer that requires a verified commit.

LunarForge's original 0.2.0 build wrote abbreviated IDs. Existing schema-1
evidence stays readable and retains its historical `run.head`; reading status
does not rewrite or upgrade that evidence. After installing a build with full
IDs, run verification again before requiring exact full-ID equality. Consumers
supporting older builds must explicitly resolve abbreviations in that repository
or reject them with an actionable request to upgrade and reverify.

A passing result proves that all configured checks passed and that LunarForge's
source fingerprint was unchanged between its before/after observations. It can
describe a dirty working tree. For a gate requiring a clean verified commit,
also require:

- Explicit `false` for both `repository.dirty` and `run.dirty`.
- The same full commit ID in `repository.head`, `run.head`, and the expected
  consumer task or checked-out commit.
- Equal, nonempty `repository.fingerprint`, `run.fingerprint`, and
  `run.final_fingerprint`, with both top-level and run states `pass`.
- Passing results for the expected check IDs, if the consumer owns the policy.

Commit first, then verify. A later commit, staging operation, or source edit
can invalidate evidence. Freshness is an observation, not a lock on future edits.
The fingerprint covers HEAD, tracked diffs, and non-ignored untracked content,
excluding LunarForge artifacts. It does not certify external services, ignored
dependencies, environment variables, or the trustworthiness of local evidence.

## Configuration and evidence boundaries

Configuration is discovered by searching upward from the process working
directory for `.lunarforge.yml`. Keep it at the repository root: its containing
directory is the execution root. `LUNARFORGE_CONFIG` and a `--config` override
are not supported. Environment variables do not select an alternate policy.

`status` validates the configuration and saved evidence but never runs checks.
Keep `.lf/` local, with the ignore file generated by `lf init` committed.
Do not parse `summary.md` or use the stored `evidence.json` as a substitute for
`status --json`: the on-disk record has a different shape and cannot establish
current freshness by itself.

## Compatibility and executable examples

`schema_version` identifies the CLI contract, independently of the executable's
version and the policy's `version`. Consumers should accept additional object
fields, avoid depending on object-key order or human messages, and reject
unsupported schema versions. Removing required fields or changing their types
or meanings requires a new schema version. New reason codes may be additive;
consumers must handle them within the known top-level state.

The [complete response fixtures](../examples/contracts/README.md) cover the
common outcomes and are suitable for consumer parser tests. They use illustrative
paths, timestamps, and fingerprints; they are not evidence for a real repository.

`go test ./cmd/lf -run 'Test(CLIConsumerContract|PublishedContractFixtures|ResultSchema|ConfigSchema)'`
builds the actual CLI, exercises its process exit/stdout boundary, validates live
responses and fixtures against the schemas, and checks full commit identity and
freshness semantics. The executable scenarios use POSIX shells and are skipped
on Windows; schema tests still run there. The JSON Schema validator is a test
dependency and is not linked into the `lf` binary.

For a complete consumer setup, see [Yuheng](yuheng.md).
