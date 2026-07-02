# LunarForge — Project Review & Recommendations

*A code review of the LunarForge MVP (July 2026), covering strengths, weaknesses,
and a suggested long-term direction.*

## TL;DR

LunarForge is a well-built, well-scoped MVP with unusually good discipline about
what it *isn't*. The code quality is high and the core idea — evidence bound to a
diff hash, with an AI agent that never gets to declare its own success — is sound
and differentiated. Its two biggest risks are not technical:

1. The repo doesn't yet dogfood itself (no `.lunarforge.yml`, no CI).
2. The tool's value depends on friction being near-zero, and setup friction is
   what will decide whether it's still in daily use in six months.

---

## Strengths

**The core invariant is the right one.** Binding evidence to
`HEAD + tracked/staged diff + status` (`internal/gitutil/gitutil.go`) and making
staleness automatic is the real insight. Most "run my checks" wrappers stop at
"did it pass"; LunarForge answers "did it pass *for this exact code*." That's
exactly the gap AI-assisted coding opens up, since agents keep editing after
checks run.

**"The agent never declares success" is enforced structurally, not by prompt.**
`lf repair` and `lf loop` re-read evidence from disk after the agent runs rather
than trusting agent output. The readiness decision is centralized in one small
type (`internal/evidence/readiness.go`) shared by `lf status` and the pre-push
hook, so the loop and the gate can't disagree.

**Code quality is well above typical MVP level.** Small packages with single
responsibilities, doc comments that explain *why* (e.g. why `Repair.Enabled` is
a pointer, why the diff hash length-prefixes sections), every package tested,
one external dependency, and careful details throughout:

- Prompts are saved to disk *before* invoking agents, so a crashed agent still
  leaves you the prompt.
- Foreign pre-push hooks are backed up instead of clobbered; `core.hooksPath`
  is honored.
- Log truncation keeps the tail, where build/test errors actually live.
- The repair prompt is rebuilt per attempt from the latest evidence rather than
  reused stale.

**The fixtures are excellent.** Three self-contained fixtures prove the
verify/repair/loop paths with zero external dependencies (no Node, no Claude) —
that's how you make a tool like this reviewable and testable.

**The README is honest.** Known limitations (untracked file contents,
`--no-verify` bypass) are stated plainly rather than hidden.

---

## Weaknesses

### 1. It doesn't dogfood itself — and has no CI

There is no `.lunarforge.yml` in this repo, no `scripts/verify.sh`, and no
GitHub Actions workflow. For a tool whose entire pitch is "verification with
evidence," this is the most glaring gap — both practically (nothing gates the
repo's own PRs) and rhetorically (it's the first thing a skeptical user checks).

**Fix:** the very next commit should add an `.lunarforge.yml` running
`go build ./... && go test ./... && gofmt -l .`, plus a minimal GitHub Actions
workflow.

### 2. The evidence is trust-based, and the repair agent can forge it

`.lf/` is excluded from the diff hash (necessarily), evidence is plain unsigned
JSON, and the repair agent runs with file-write permissions in the repo. A
confused or adversarial agent could simply write a passing `evidence.json` and
update `.lf/latest`, and the pre-push gate would wave it through. The prompt
says "don't," but the project's own philosophy is that prompts ask and
enforcement verifies.

**Options, in increasing strength:**

- Store evidence outside the working tree (e.g. `~/.lunarforge/<repo-id>/`) —
  this also stops agents from *reading* and imitating the format.
- HMAC-sign evidence with a key kept outside the repo.

This matters more as `lf repair` gets used with more capable agents.

### 3. No timeouts or cancellation anywhere

A hung test or a hung agent blocks `lf verify` / `lf repair` forever — there's
no `context.Context`, no per-command `timeout:` config, and no signal handling.
For a tool meant to run unattended inside `lf loop` with an AI agent in the
middle, a hang is a likely failure mode.

**Fix:** per-command and per-agent timeouts (config keys with sane defaults).
First code change after dogfooding.

### 4. Run-ID collisions at 1-second resolution

`NewRunID` formats to whole seconds (`internal/evidence/evidence.go`), and
`lf loop` runs verify repeatedly in quick succession during repair. Two verify
runs in the same second share a run directory and silently overwrite each
other's evidence. Fast fixtures make this reachable today.

**Fix:** sub-second precision or a collision suffix.

### 5. Dropped error message in the runner

In `failRecord` (`internal/runner/runner.go`), the failure message is discarded
(`_ = msg`) — if creating the stdout/stderr file fails, the result is a failed
command record with exit code −1 and no explanation anywhere.

### 6. The repair loop can't tell a no-op agent from a trying one

With a no-op agent (or a real agent that gives up), the loop burns all N
attempts running the full verify suite each time. Comparing the diff hash
before/after an attempt would allow an early stop with "agent made no changes."

### 7. Evidence grows forever

`.lf/runs/` has no retention. A `lf clean --keep N` (or auto-prune on verify)
is cheap and prevents the "why is my repo directory 2 GB" moment.

### Smaller notes

- The pre-push hook checks the *working tree's* readiness, not the specific
  refs being pushed (the hook ignores its stdin), so pushing a non-checked-out
  branch is gated by the wrong evidence.
- `lf verify` has no `--json` while `lf status` does.
- There's no way to re-run just one failed verify command.

---

## Long term: how it becomes genuinely useful day-to-day

The honest competitive question is: why is this better than "run `npm test` +
CI"? The answer is the staleness binding and the agent loop — but only if the
friction is near zero. Suggested sequence:

1. **Dogfood first** — this repo, then other active repos. The real papercuts
   will surface within a week, and those findings should drive the roadmap more
   than the current README roadmap does.

2. **Kill setup friction with smart `lf init`.** Detect `package.json` /
   `go.mod` / `CMakeLists.txt` / `Cargo.toml` and generate a working config
   with real commands, instead of pointing at a `verify.sh` the user must
   write. The first-run experience should be `lf init && lf verify` succeeding
   in a real repo with zero editing.

3. **Integrate with Claude Code hooks.** The highest-leverage move for a real
   AI-assisted workflow: a Claude Code `Stop` hook that runs `lf verify`
   automatically when an agent session ends means evidence gets recorded
   without anyone remembering anything — and the pre-push gate then has teeth
   for free. An `lf hooks claude` subcommand that writes the
   `.claude/settings.json` entry would make LunarForge effectively invisible,
   which is what daily-driver tools need to be. (An MCP server exposing
   `status`/`verify` so agents can query readiness themselves is the step
   after that.)

4. **Build the CI mirror** (already on the roadmap — ranked after the hooks
   integration). `lf ci generate` emitting a GitHub Actions workflow from
   `.lunarforge.yml` closes the `--no-verify` loophole and makes the config the
   single source of truth for local and remote checks.

5. **Then evidence hardening** (out-of-tree storage or signing, per weakness
   #2) — it matters once repair sees real use.

**What *not* to pursue:** the "autonomous implementation from vague tasks"
roadmap item. Claude Code and Codex already own that layer and are moving fast;
LunarForge's durable niche is being the small, boring, trustworthy verification
layer *underneath* whichever agent wins. The project's discipline about staying
narrow is its best strategic asset — keep it.

---

## Suggested order of work

| Priority | Item | Size |
|---|---|---|
| 1 | Dogfood: add `.lunarforge.yml` + verify script + GitHub Actions to this repo | Small |
| 2 | Fix run-ID collisions (sub-second precision) | Small |
| 3 | Fix dropped error message in `failRecord` | Trivial |
| 4 | Per-command / per-agent timeouts | Medium |
| 5 | Smart `lf init` (project-type detection) | Medium |
| 6 | Claude Code hooks integration (`lf hooks claude`) | Medium |
| 7 | Early-stop repair when the agent makes no changes | Small |
| 8 | `lf clean` / evidence retention | Small |
| 9 | `lf ci generate` (remote mirror) | Medium |
| 10 | Evidence hardening (out-of-tree storage or signing) | Large |
