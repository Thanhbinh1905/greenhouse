# Implementation sequence

This sequence covers the approved SPEC.md without treating the first executable slice as the full product. Each later step needs its own bounded Change Brief and exact-head independent review.

## 1. Lifecycle and settings: delivered for review

Areas A, B, E, F, and D1: one Linux Go executable, per-user Unix daemon, singleton ownership before recovery, canonical repository/worktree slots, retained runs, version-1 JSON requests and storage, action receipts, current-token checks, immutable input/configuration snapshots, retained usage, Settings Apply versus Save, and explicit New run.

Runs remain at intent/unconfigured. Target is an empty unconfigured identity, not an inferred default branch. Submitted head and branch remain immutable; candidate initially equals submitted head. Rule selection is distinct from loading. Source mismatch blocks old-run advancement. Explicit New can capture a moved source only without a writer or unknown-outcome hazard. Synthetic interrupted-writer tests park unknown outcomes; no executor is claimed.

The implementation uses a single serialized daemon and atomic whole-store replacement rather than a database or generic workflow engine. Limits and compatibility assumptions are in README.md. No dependency on an orchestrator is required.

Still unsupported within these areas: live event subscriptions, explicit recovery, and an embedding contract. Task t67 adds configuration migration and repository trust. Task t7 adds finding views, execution summaries, and cancellation. Terminal clients refresh through Inspect rather than receive live events. Settings cannot start work.

## 2. Rule snapshots and independent review: delivered for review

Areas B, C, D review, and F: declarative pack schema, local-file loading with digests and provenance, required-pack failure, separate reviewer criteria and repair guidance, exact emitted-input capture, backend support matrix, read-only fresh reviewer contexts, finding identity/coverage, independent verdicts, and review-budget consumption.

Task t4 implements version-1 declarative built-ins and explicit absolute-file selections, token-bound resolution, immutable exact-byte snapshots, required-load eligibility, AXI inspection, and terminal resolution and criteria views. Version-1 lifecycle stores remain readable through additive optional fields without resetting counters. The fixture-only input writer captures reviewer and fixer content identities with shared criteria and role-specific guidance. See [the rule contract](rules.md).

Task t7 adds explicit intent and target preparation, trusted per-user Pi configuration, asynchronous one-pass dispatch, strict structured coverage and findings, retained finding identity, exact emitted-input provenance, cancellation, and admission-time review usage. The daemon persists execution intent before launch and parks unknown outcomes after restart. Executable fake-Pi fixtures establish invocation and delivery, not live provider compliance. Assess model compliance separately with recorded provider-backed development evaluations. Do not count selecting a pack as loading it or a backend's claimed read-only mode as OS confinement.

## 3. Private-worktree one-pass repair and guarded handback: initial slice

Area D one-pass, E, and D2: AXI and terminal expose explicit selected-finding repair and separate handback actions. Admission consumes one durable optional repair budget, binds the exact review/intent/candidate/rules/selection and trusted fixer/check configuration, creates a private candidate worktree, and invokes a fresh restricted Pi tool set. The controller runs configured argv checks, validates candidate scope, commits only selected paths, and records the candidate head as fixed but not independently rereviewed. Handback fast-forwards only an unchanged, clean source worktree after verifying exact candidate lineage and stopped writer identity. Dirty source work blocks handback. Unpublished custody blocks New run.

The red-before executable test rejected `repair` as `json: unknown field "selected"`. The green executable/real-Git fixtures prove one selected repair, immutable rule delivery, passing and failing configured checks, local commit identity, retained failed candidate and budget, source dirty preservation, dirty-candidate and moved-source handback refusal, safe fast-forward handback, interrupted-handback reconciliation, replay idempotency, no second review, and restart evidence retention. Runner and daemon tests cover descendant cleanup, live fixer/check cancellation, killed-runner ambiguity, and restart after an active-writer crash. A completed repair commit also survives an abrupt daemon restart.

The slice rejects missing trusted checks and unsupported Git hooks, filters, signing, binary/submodule changes, and non-clean source handback. Executable real-Git regressions prove missing checks refuse admission without spending repair budget, changed check authority refuses before execution or commit, and hook/signing hazards refuse before fixer execution. A fake Git wrapper kills the real daemon immediately after the candidate commit; restart preserves candidate refs and consumed budget, parks the outcome as unknown, and does not rerun the fixer, check, or commit. Recovery after interruption parks unknown writer outcomes without retry or cleanup. The 56/100-column PTY exercise covers selected repair, check results, dirty handback refusal, safe handback, and AXI parity. A real linked-worktree race test verifies the shared repository lock admits only one writer. Broader stale-action coverage and interruption points outside the tested commit boundary remain unsupported. This is not complete acceptance of SPEC.md.

## 4. Bounded full pipeline, authority modes and forge delivery

Task t18 delivers the local half of area D. A private per-user `pipeline.json` supplies test, lint and doc commands, not-applicable reasons, authorized gate repairs and a gate-repair limit. The `pipeline` action runs intent, target synchronization, independent review, test, documentation and lint on a managed private candidate and ends at `locally-ready`. Commands run through the subreaper command runner with exit code, output tail, argv and executable digest. Target synchronization merges the exact target with Git and never rebases; a conflict opens a gate for the author or an explicitly authorized fixer. Missing commands are `unconfigured`. Documentation requires the reviewer's impact assessment as well as passing doc commands. In `auto` mode, authorized gate repairs use the one-pass writer custody, consume a separate budget and create a new candidate. Evidence binds declared inputs, so a new candidate reruns review and every gate. Guarded handback fast-forwards the source to a locally ready candidate.

Executable fake-Pi, fake-command and real-Git tests cover agent-free command stages, missing commands, bounded repair failures, invalidation by candidate and settings changes, rerun after repair, target conflicts, cancellation, and restart and reattach without duplicate work. The PTY check covers 56 and 100 columns.

Task t20 adds authority modes and review-cap outcomes. Ask groups the actionable findings of a review into one `findings` decision and asks before gate repairs; informational findings stay visible without a question. Auto repairs findings and authorized gates. YOLO adds a configured actor from `pipeline.json` that also answers the cap decision with `final-fix` or `accept-head`. Repairs that delete files, change file modes, leave the authorized paths, or that the fixer escalates as intent, acceptance or credentials changes park a user-only `authority` decision in every mode. The combined cap counts review and finding-repair rounds separately and as a total; gate retries count apart. The `decide` action answers accept-head, final-fix, increase-cap, rethink and discard with token-bound, replayable receipts that record the actor and reason. Accept-head binds the exact candidate and review inputs and never passes a failing gate. Final-fix is a separate, restart-safe, single grant. Rethink links a successor run; discard keeps every record and ref. Executable fake-Pi and real-Git tests cover each mode, user-only refusal, cap exhaustion, cap increase with retained usage, acceptance binding, final-fix replay and restart, rethink, discard, and stale or duplicate answers. The PTY check answers decisions at 56 and 100 columns.

Step 4 still leaves unsupported: mechanical lint autofix and recovery of interrupted pipeline steps. Task t67 adds expanded fixer authority grants.

Delivery scope in step 4:

Area D and standalone E: the trusted command plan can configure a Git remote and GitHub repository. The controller probes and pushes the candidate ref without force, reconciles an already-pushed exact head, and creates or updates one branch PR. It checks only trusted required GitHub check identities for the exact candidate SHA. It persists the PR number and URL, verifies that PR still points to the candidate during CI polling, and rejects head movement. Missing, duplicate, pending, failed, stale and unknown required checks block readiness; ancillary green checks do not count. Transport retries use persisted stage attempts, exponential backoff and a three-attempt limit; exhaustion parks without dispatching a fixer. Push command-runner evidence remains in the stage result. Authorized CI repairs are bounded and invalidate earlier evidence. Delivery does not dispatch an agent and never merges. Local fixtures cover CI-ready, failed CI repair, check identity validation, restart reconciliation, and completed delivery in 56- and 100-column PTYs. The GitHub adapter accepts a local API base URL for isolated fixtures.

## 5. Reviewer and fixer model selection

Settings and AXI carry independent exact provider-qualified reviewer and fixer model selections. Pi is the default backend; the default reviewer is `openai/gpt-5.6-luna`, based on the measured comparison in `docs/evidence/review-eval/README.md`. Model selections do not grant executable authority. Greenhouse discovers Pi models through AXI, supports trusted per-user provider endpoints in `providers.json`, reads API keys only from named environment variables, and preflights provider model access before dispatch. Pi receives only a temporary provider file that references the environment variable. A product-path smoke against the recommended default model passed through the configured proxy.

Task t40 adds Claude Code and Codex as reviewer and fixer backends beside Pi, classifies backend quota, login and availability failures, parks the affected stage without spending budget, and adds resume, rerun, rerun from start, automatic resume, fallback and agent switching to the terminal and AXI. See README.md and `docs/evidence/backend-smoke.md`.

## Live standalone validation (t59)

The [recorded live runs](evidence/v1-live/README.md) prove standalone full delivery and selected one-pass repair. Claude Code `claude-sonnet-5-5` reviewed an accuracy change, Pi repaired its empty-input regression, and Claude independently rereviewed the committed candidate. Test, docs, lint, push, PR creation, and exact-head GitHub Actions passed. The full run stopped at `ci-ready` with `repaired-pass`; PR #2 remains open and unmerged in the private validation repository.

A fresh isolated HOME installed Greenhouse and ran the bare-command Pi one-pass flow. One review and one selected repair produced a checked local commit. Dirty-source handback refused; clean handback succeeded. The result remained `fixed-not-independently-rereviewed`, with no push or PR.

The live exercise exposed empty-diff rereview refusal when a repair restores the target tree. The fix retains the preceding paths, captures current committed contents, and invokes independent review again. Word wrapping now covers screens and prompts at 56, 80, and 100 columns. Only long finding IDs are abbreviated, and a unique displayed finding ID is selectable; URLs, refs, and other long values are printed whole on their own line. Cancelled repair progress now remains `cancelling` until final completion, avoiding a stale-token window on rerun. The unused `runCaptured` stdout return is removed. The AXI test helper disables Go race-runtime exit delay so scheduled-retry snapshots do not arrive one second late.

V1 is not complete against every SPEC.md item. The missing work is listed below. This validation does not implement the Posse adapter, repository trust, event subscriptions, or recovery of unknown writer outcomes.

## Versioned configuration, repository trust, fixer grants and transcripts (t67)

Areas B, D and F. Per-user configuration files carry schema version 2; unversioned files are version 1 and stay unused until the explicit `migrate` action rewrites them losslessly with a backup and a recorded migration. Newer versions refuse. `pipeline.json` names `budget_unit` `rounds`, and no other unit is accepted.

A repository's `.greenhouse.json` applies to new runs only after the User trusts its exact digest. It can only narrow or select among per-user choices; authority keys make it ignored, with the reason shown. Each snapshot records the source of every value, and Save can target the trusted override.

Repairs that add or delete files park a user-only decision. The User can grant exactly the attempted paths for the run, or declare scoped grants in `pipeline.json`; both are recorded. Reviewer and fixer output goes to a redacted, 4 MiB per-run transcript log that AXI `transcript` and terminal `o` show.

Executable daemon, AXI and real-Git tests cover each item; the PTY check covers 56, 80 and 100 columns. Before and after captures are in `evidence/config-trust/`.

## Review snapshot, resolution responses and OS confinement (t68)

Areas C and F. Each review binds a digest of the candidate's full tree listing into its input. The reviewer runs in a read-only copy of that tree written from the object store, and Greenhouse verifies the copy after the run; any change makes the review inconclusive.

A finding repair must answer every selected finding as fixed, disputed with a reason, or deferred under a cited answered decision. Invalid responses refuse the repair. In the full pipeline the next independent review checks each response. Only a confirmed response resolves a finding, and a dispute never counts as a fix. A repair that only disputes or defers commits nothing and still triggers that review.

On Linux, reviewers and fixers run under Landlock. Reviewers write only their backend's private state, fixers also their candidate and scratch directory, and TCP is limited to the backend's provider, HTTPS and proxy ports. Without Landlock, or below ABI 4, the confinement record is `degraded` and the terminal and AXI show it.

Executable daemon, AXI and terminal tests with real Git and fake backends cover each item, including a seccomp-filtered daemon without Landlock. PTY captures at 56, 80 and 100 columns are in `evidence/review-confinement/`. The eval rerun, `evidence/review-eval/11-snapshot-confinement`, kept 100% recall and 81% precision.

## 6. Live backend validation and host contract

Areas C, E, and F: validate selected real agent and forge backends, capture actual rule inputs and permissions, and publish versioned configuration/AXI compatibility guarantees. Verify custody, budgets, and stale evidence at embedding boundaries.

The Posse adapter belongs in the Posse Project. Embedded execution must use its author, candidate custody, grants, publication, and CI owner rather than start a competing standalone pipeline.

## Unsupported spec behavior in this slice

- Manual acceptance: the recorded bare-command sessions use scripted real PTYs, accepted as proof for this Task. A person has not manually repeated them; the [manual checklist](evidence/v1-live/manual-acceptance.md) is available for later use.

- Area A: reconciliation of orphan executors and authorized recovery. Unknown outcomes park without retry or PID-based signalling.
- Area C: OS confinement on platforms other than Linux, where the confinement record is `degraded`. Provider-backed review evaluation is in `eval/`; live fixer evidence covers the two standalone flows, without a broader evaluation corpus.
- Area D: cap decisions for embedded hosts.
- Area E and D2: explicit recovery for unknown candidate outcomes, persistent cross-daemon ref-lease reconciliation, broader provider-backed fixer evaluation. Live exact-head review and GitHub delivery are recorded above.
- Area F: event subscriptions, recovery, and the Posse adapter. AXI and the terminal share findings, bounded summaries, cancellation, and current-token actions. There is no fake successful gate or dummy approval.
