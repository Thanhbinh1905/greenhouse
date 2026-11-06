# Lifecycle slice verification

## Results

- `go test ./...` passed. Executable fixtures cover concurrent first attachment, aliases, distinct linked worktrees, retained runs, Apply/Save/New semantics, replay, action-ID collision, stale tokens, source movement, dirty files, unsupported operations, singleton ownership, disconnect, restart, interrupted writers, retained budget usage, and damaged storage.
- `go test -race ./...` passed. Fixtures also build the actual client/daemon executable with `-race`.
- `go vet ./...` passed.
- `test -z "$(gofmt -l *.go)"` passed.
- `python3 tools/terminal_check.py "$TMPDIR/greenhouse" docs/evidence` passed with a real built executable in 56- and 100-column PTYs. It exercised Settings, rule selection, Apply, Save, New run, detach, reattachment, and AXI state comparison.

`terminal-56.log`, `terminal-100.log`, and `terminal-56-reattach.log` contain captured terminal interactions. The capture normalizes carriage returns and strips trailing spaces; it retains screen-clear escapes and command echoes. The PTY test checks emitted home-page line widths. It does not substitute source matching for interaction.

## Red-before evidence

1. The first executable attachment test failed before product code existed: `build: greenhouse: no non-test Go files`. After implementing the executable/daemon/store path, `TestConcurrentAttach` passed.
2. A damaged-run fixture with missing configuration snapshots was accepted by the daemon before validation: `damaged store accepted`. The bounded command timed out because the invalid daemon kept serving. After startup validation, `TestDamagedStoreRefusedBeforeBinding` passed and proved evidence was not rewritten.
3. The 56-column PTY check failed because long blocked-reason lines exceeded terminal width. After terminal-width-aware wrapping, the same executable interaction passed.
4. The PTY rule-selection flow failed its applied-state assertion before the terminal supported editing rule selections. After adding that draft field, Apply and subsequent Save/New assertions passed.

5. `TestRestartAndUnknownWriter` failed with `unsafe action advertised` while the envelope still offered New/Apply for a parked unknown writer. Eligibility filtering removed those actions, and the test passed.

The remaining lifecycle checks were added after the initial implementation and passed on their first run. They provide executable regression coverage, not separate red-before evidence for every acceptance item. This is a limitation of the test-first process followed in this turn.

## Evidence boundaries

All Git fixtures and state homes are isolated beneath the Task scratch directory. Synthetic persisted `writing` state and nonzero usage exercise recovery/accounting without invoking an agent. No review, test-stage executor, repair, push, PR, CI, or acceptance evidence is fabricated.

Source observations use read-only Git commands. No product command creates a worktree, changes refs, edits source files, or invokes a backend. This Task creates local commits only; SPEC.md and LEAD_BRIEF.md remain unchanged. No remote, installed tool, Posse configuration, or other Project was changed.

Independent Review round 1 requested changes at `17a2b6c95a455976d7f615e03d30b45db4fe099a`. The follow-up below addresses both findings and requires exact-head rereview before admission. This document records author verification, not an independent verdict.

## Review round 1 fixes

- Git context leakage reproduced through the built executable: `TestDaemonGitEnvironmentDoesNotSelectOtherRepository` failed with `repository B attached to A's run`. The daemon inherited A's `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, namespace, and injected configuration. Git identity commands now discard inherited `GIT_*` variables before resolving the invoking path. The same test passes for A, B, a linked worktree, and daemon restart with the polluted environment.
- Explicit replacement after a commit reproduced through AXI: `TestNewRunAfterSourceMovement` failed with `safe New refused after source movement` and `source head or branch moved`. New now captures the current source identity when its run/input/gate/token is current and no writer or unknown-outcome hazard exists. Apply and Save remain blocked on the old run. Tests prove stale-token refusal, unchanged old evidence, saved-default inheritance, duplicate New replay, branch movement, and durable replacement across restart. Unknown-writer replacement still refuses even after source movement.
- The real 100-column PTY flow now creates a source commit between Save and Inspect/New. The terminal displays blocked old-run advancement, then creates a new run at the changed head. Narrow reattachment and AXI confirm that identity. The regenerated terminal logs capture this interaction.
- After both fixes: `go test ./...` passed in 51.570s; `go test -race ./...` passed in 52.639s; vet, formatting, and the executable PTY check passed.

The review's referenced scratch reproduction script was unavailable at `/home/thanhbinh/.posse/scratch/greenhouse/t2/repro.py` (ENOENT). The executable regressions above reconstructed both reproductions from the review report. No reviewer artifact was modified.

## Risks and rollback

The daemon serializes requests and rewrites the whole JSON store. It is appropriate for this bounded lifecycle slice, not high-volume execution. A slow client can occupy the server until the five-second request deadline. Unix socket length limits require a short state home. Terminal interaction is line-oriented, with no live refresh subscription. Pipeline and backend behavior remains unsupported as listed in `docs/implementation-plan.md`.

Rollback: revert the local implementation commit before admission. Preserve any isolated state directory that contains evidence; do not delete or reinterpret it to downgrade schemas. Stop only the daemon whose state directory and PID you have verified.

Friction: this Pi launch exposes no tracked background-command tool. Verification ran in the foreground, without detached polling processes. The first long-name test fixture exceeded the Unix socket path limit; fixtures now use a short scratch prefix. Neither issue required changing Posse or installed integrations.
