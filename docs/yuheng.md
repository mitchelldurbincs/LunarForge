# LunarForge with Yuheng

[Yuheng](https://github.com/mitchelldurbincs/yuheng) displays workflow evidence
and can explicitly dispatch verification. LunarForge owns check execution and
snapshot-bound results. Yuheng owns workflow state, task isolation, approvals,
and retry decisions.

## Display existing evidence

1. Install `lf` and make it available on the Yuheng service's PATH, or set
   `lf_binary` to the installed binary's absolute path. A terminal's PATH may
   differ from the service's PATH.
2. In the target repository, run `lf init`, edit `.lunarforge.yml`, and commit
   the policy, `.lf/.gitignore`, and the candidate source changes.
3. Run `lf verify --json --quiet`. Verify after committing so the evidence
   describes the clean commit that Yuheng observes.
4. Enable the provider in Yuheng's external `config.yaml`:

   ```yaml
   modules:
     lf: true
   lf_binary: lf
   ```

5. Add this stage to the repository's Yuheng workflow:

   ```yaml
   - id: verify
     label: Local checks
     provider: lf
     evidence: gates
     per_commit_gates: true
   ```

Use `evidence: check:<id>` to display an individual configured check. The passive
provider only runs `lf status --json`; viewing or refreshing the dashboard does
not execute repository checks.

Yuheng's optional per-repository `lf_config` setting passes
`LUNARFORGE_CONFIG`. LunarForge does not consume that variable. Leave the setting
unset and use the repository-root `.lunarforge.yml`.

## Explicit native verification

Yuheng also supports an independently configured native verification operation.
It copies task input into a disposable repository, writes reviewed verification
commands as that copy's policy, and runs `lf verify` followed by `lf status --json`.
It then binds successful evidence to its task input. This is separate from the
passive provider and requires explicit Yuheng configuration.

Follow Yuheng's [native operations guide](https://github.com/mitchelldurbincs/yuheng/blob/main/docs/native-operations.md)
for the current operation settings, including reviewed `build` and `tests`
command arguments. Check commands execute with the caller's privileges;
LunarForge itself does not provide a sandbox.

New LunarForge runs emit full commit IDs, matching the native verifier's exact
comparison with `git rev-parse HEAD`. Original 0.2.0 runs used abbreviated IDs.
After upgrading, recreate verification evidence; historical records retain their
abbreviated IDs. A disposable native verification attempt must use the updated
binary when creating its new evidence.

## Troubleshooting and consumer tests

| Observation | Next action |
|---|---|
| Binary unavailable | Check the service PATH or absolute `lf_binary`, permissions, and working directory. |
| `blocked / no_evidence` | Run verification in the repository being observed. This valid envelope has no `run` or `checks`. |
| `pass` but a Yuheng gate is pending | Check both dirty booleans and the expected commit; commit and reverify if needed. |
| `stale / snapshot_changed` | Reverify the final candidate; a historical passing run is not current evidence. |
| Native verification rejects the captured HEAD | Check full versus abbreviated IDs and which `lf` binary the operation invokes. |
| `blocked / tool_unavailable` or `timed_out` | Fix the environment or time budget before attempting code repair. |
| `error / evidence_corrupt` | Preserve the run for diagnosis, then create new evidence. |

At the time of this integration, Yuheng's passive provider rejects envelopes
without a check list before inspecting `no_evidence`. That can display a valid
empty-history response as unavailable. The fix belongs in the consumer:
interpret top-level state/reason before requiring `run` or `checks`.

For Yuheng's own CI, use the [response fixtures](../examples/contracts/README.md)
for parser cases and run a pinned real LunarForge binary against a temporary Git
repository. Cover initial missing evidence, clean pass, failed/skipped checks,
an edit after a pass, a blocked check, and corrupt evidence. Exercise the native
operation with the same binary to catch commit-ID assumptions. LunarForge's tests
cover its executable contract; they do not run the Yuheng application.
