# Implementation sequence

This sequence covers the approved SPEC.md without treating the first executable slice as the full product. Each later step needs its own bounded Change Brief and exact-head independent review.

## 1. Lifecycle and settings: delivered for review

Areas A, B, E, F, and D1: one Linux Go executable, per-user Unix daemon, singleton ownership before recovery, canonical repository/worktree slots, retained runs, version-1 JSON requests and storage, action receipts, current-token checks, immutable input/configuration snapshots, retained usage, Settings Apply versus Save, and explicit New run.

Runs remain at intent/unconfigured. Target is an empty unconfigured identity, not an inferred default branch. Submitted head and branch remain immutable; candidate initially equals submitted head. Rule selection is distinct from loading. Source mismatch blocks old-run advancement. Explicit New can capture a moved source only without a writer or unknown-outcome hazard. Synthetic interrupted-writer tests park unknown outcomes; no executor is claimed.

The implementation uses a single serialized daemon and atomic whole-store replacement rather than a database or generic workflow engine. Limits and compatibility assumptions are in README.md. No dependency on an orchestrator is required.

Still unsupported within these areas: live event subscriptions, explicit recovery, repository overrides/trust enrollment, migration, and an embedding contract. Task t7 adds finding views, execution summaries, and cancellation. Terminal clients refresh through Inspect rather than receive live events. Settings cannot start work.

## 2. Rule snapshots and independent review: delivered for review

Areas B, C, D review, and F: declarative pack schema, local-file loading with digests and provenance, required-pack failure, separate reviewer criteria and repair guidance, exact emitted-input capture, backend support matrix, read-only fresh reviewer contexts, finding identity/coverage, independent verdicts, and review-budget consumption.

Task t4 implements version-1 declarative built-ins and explicit absolute-file selections, token-bound resolution, immutable exact-byte snapshots, required-load eligibility, AXI inspection, and terminal resolution and criteria views. Version-1 lifecycle stores remain readable through additive optional fields without resetting counters. The fixture-only input writer captures reviewer and fixer content identities with shared criteria and role-specific guidance. See [the rule contract](rules.md).

Task t7 adds explicit intent and target preparation, trusted per-user Pi configuration, asynchronous one-pass dispatch, strict structured coverage and findings, retained finding identity, exact emitted-input provenance, cancellation, and admission-time review usage. The daemon persists execution intent before launch and parks unknown outcomes after restart. Executable fake-Pi fixtures establish invocation and delivery, not live provider compliance. Assess model compliance separately with recorded provider-backed development evaluations. Do not count selecting a pack as loading it or a backend's claimed read-only mode as OS confinement.

## 3. Private-worktree one-pass repair and guarded handback: initial slice

Area D one-pass, E, and D2: AXI and terminal expose explicit selected-finding repair and separate handback actions. Admission consumes one durable optional repair budget, binds the exact review/intent/candidate/rules/selection and trusted fixer/check configuration, creates a private candidate worktree, and invokes a fresh restricted Pi tool set. The controller runs configured argv checks, validates candidate scope, commits only selected paths, and records the candidate head as fixed but not independently rereviewed. Handback fast-forwards only an unchanged, clean source worktree after verifying exact candidate lineage and stopped writer identity. Dirty source work blocks handback. Unpublished custody blocks New run.

The red-before executable test rejected `repair` as `json: unknown field "selected"`. The green executable/real-Git fixtures prove one selected repair, immutable rule delivery, passing and failing configured checks, local commit identity, retained failed candidate and budget, source dirty preservation, dirty-candidate and moved-source handback refusal, safe fast-forward handback, interrupted-handback reconciliation, replay idempotency, no second review, and restart evidence retention. The bounded process-runner tests cover descendant cleanup after normal completion and cancellation.

The slice rejects missing trusted checks and unsupported Git hooks, filters, signing, binary/submodule changes, and non-clean source handback. Progress and execution evidence persist in shared AXI/terminal state. Recovery after interruption parks unknown writer outcomes without retry or cleanup. The 56/100-column PTY exercise now covers selected repair, check results, dirty handback refusal, safe handback, and AXI parity. A complete crash-injection matrix, live fixer/check cancellation and descendant tests at the daemon level, simultaneous linked-worktree executor races, and the stale selected-action matrix remain unverified. A linked-worktree admission test confirms existing unpublished repair custody blocks another repair. This is not complete acceptance of SPEC.md.

## 4. Bounded full pipeline and forge fixture

Area D and standalone E: trusted intent/command plan, target synchronization, deterministic tests/lint, documentation assessment, bounded repair/CI retries, changed-input invalidation, ask/auto/YOLO authority, combined cap accounting, exact-head accept-head, final-fix/final Review, rethink lineage, and discard provenance.

Add guarded push, idempotent PR creation/update, and exact-head checks against a forge fixture. Stop at CI-ready without merging. Prove that command and forge work do not dispatch agents unnecessarily. No green state can substitute for missing commands or unknown checks.

## 5. Live backend validation and host contract

Areas C, E, and F: validate selected real agent and forge backends, capture actual rule inputs and permissions, and publish versioned configuration/AXI compatibility guarantees. Verify custody, budgets, and stale evidence at embedding boundaries.

The Posse adapter belongs in the Posse Project. Embedded execution must use its author, candidate custody, grants, publication, and CI owner rather than start a competing standalone pipeline.

## Unsupported spec behavior in this slice

- Area A: reconciliation of orphan executors and authorized recovery. Unknown outcomes park without retry or PID-based signalling.
- Area B: raw transcript logs, repository trust, and overrides. Findings and bounded execution summaries are available.
- Area C: provider-backed behavioral evaluations, full-tree review snapshots, explicit finding resolution, and OS confinement. Pi review is patch-based with fresh context and read-only tools.
- Area D: pipeline stages other than independent review and explicitly authorized one-pass repair/check/commit; operational ask/auto/YOLO decisions; automatic retries; acceptance, final-fix, rethink, and discard.
- Area E and D2: publication, forge and CI, explicit recovery for unknown candidate outcomes, persistent cross-daemon ref-lease reconciliation, cancellation of a live fixer, and independently exact-head review.
- Area F: finding-resolution responses, transcript logs, event subscriptions, recovery, and the Posse adapter. AXI and the terminal share findings, bounded summaries, cancellation, and current-token actions. There is no fake successful gate or dummy approval.
