# CLI response fixtures

These complete, illustrative schema-version-1 envelopes are validated by
LunarForge's Go tests. Import them into consumer parser tests. They are not
on-disk `evidence.json` records or live proof of verification.

| Fixture | Process exit | What it demonstrates |
|---|---:|---|
| [status-pass.json](status-pass.json) | 0 | Fresh evidence for a clean full commit ID; explicit false booleans. |
| [verify-fail.json](verify-fail.json) | 1 | Failed check output and an explicitly skipped subsequent check. |
| [status-stale.json](status-stale.json) | 1 | Current snapshot differs even though `run.state` remains `pass`. |
| [status-no-evidence.json](status-no-evidence.json) | 2 | Supported status with no `run`, `checks`, or `error` fields. |
| [verify-blocked.json](verify-blocked.json) | 2 | A timeout includes run and check detail. |
| [verify-config-invalid.json](verify-config-invalid.json) | 2 | Early configuration failure includes an error but no run. |
| [status-corrupt.json](status-corrupt.json) | 3 | Unusable evidence includes an error and incomplete repository metadata. |

See the [integration contract](../../docs/integration-contract.md) for field
presence, compatibility, current versus historical state, and clean-commit gates.
