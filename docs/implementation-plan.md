# Implementation sequence

This sequence covers the approved SPEC.md without treating the first executable slice as the full product. Each later step needs its own bounded Change Brief and exact-head independent review.

## 1. Lifecycle and settings: delivered for review

Areas A, B, E, F, and D1: one Linux Go executable, per-user Unix daemon, singleton ownership before recovery, canonical repository/worktree slots, retained runs, version-1 JSON requests and storage, action receipts, current-token checks, immutable input/configuration snapshots, retained usage, Settings Apply versus Save, and explicit New run.

Runs remain at intent/unconfigured. Target is an empty unconfigured identity, not an inferred default branch. Submitted head and branch remain immutable; candidate initially equals submitted head. Rule selection is distinct from loading. Source mismatch blocks old-run advancement. Explicit New can capture a moved source only without a writer or unknown-outcome hazard. Synthetic interrupted-writer tests park unknown outcomes; no executor is claimed.

The implementation uses a single serialized daemon and atomic whole-store replacement rather than a database or generic workflow engine. Limits and compatibility assumptions are in README.md. No dependency on an orchestrator is required.

Still unsupported within these areas: live event subscriptions, finding/log views, executor cancellation, explicit recovery, repository overrides/trust enrollment, migration, and an embedding contract. Terminal clients refresh through Inspect rather than receive live events. Settings cannot start work.

## 2. Rule snapshots and independent review

Areas B, C, D review, and F: declarative pack schema, local-file loading with digests and provenance, required-pack failure, separate reviewer criteria and repair guidance, exact emitted-input capture, backend support matrix, read-only fresh reviewer contexts, finding identity/coverage, independent verdicts, and review-budget consumption.

Test prompt delivery deterministically. Assess model compliance separately with recorded provider-backed development evaluations. Do not count selecting a pack as loading it or a backend's claimed read-only mode as OS confinement.

## 3. Private-worktree one-pass repair and guarded handback

Areas D one-pass, E, and D2: managed candidate worktrees, writer exclusion, one selected authorized repair, checks and local commit, explicit unreviewed-after-repair state, divergence/dirty-state refusal, and recovery refs. Add unknown-process/custody reconciliation with explicit recovery instead of retrying on attach.

Keep source worktrees and unpublished commits intact. Verify crash-after-commit, writer replacement, and handback refusal through the executable with instrumented fake backends.

## 4. Bounded full pipeline and forge fixture

Area D and standalone E: trusted intent/command plan, target synchronization, deterministic tests/lint, documentation assessment, bounded repair/CI retries, changed-input invalidation, ask/auto/YOLO authority, combined cap accounting, exact-head accept-head, final-fix/final Review, rethink lineage, and discard provenance.

Add guarded push, idempotent PR creation/update, and exact-head checks against a forge fixture. Stop at CI-ready without merging. Prove that command and forge work do not dispatch agents unnecessarily. No green state can substitute for missing commands or unknown checks.

## 5. Live backend validation and host contract

Areas C, E, and F: validate selected real agent and forge backends, capture actual rule inputs and permissions, and publish versioned configuration/AXI compatibility guarantees. Verify custody, budgets, and stale evidence at embedding boundaries.

The Posse adapter belongs in the Posse Project. Embedded execution must use its author, candidate custody, grants, publication, and CI owner rather than start a competing standalone pipeline.

## Unsupported spec behavior in this slice

- Area A: actual execution/cancellation, process identity reconciliation, and authorized recovery.
- Area B: findings/details/log views, actual loaded-rule content, repository trust/overrides, and in-flight instruction changes.
- Area C: pack resolution/delivery, agent adapters, reviewer isolation, finding ledger, verdicts, and behavioral evaluations.
- Area D: all nine executable pipeline stages; operational ask/auto/YOLO decisions; reviewer/repair/retry accounting beyond persisted counters; acceptance, final-fix, rethink, discard, and one-pass execution.
- Area E and D2: private candidates, ref leases, repair commits, guarded handback, publication, forge and CI, and executor recovery.
- Area F: finding responses, logs, event subscriptions, cancellation/recovery, and the Posse adapter. There is no fake successful gate or dummy approval.
