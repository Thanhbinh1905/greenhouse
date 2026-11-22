# t174: Reviewer priority definitions and repeatability

## Change

The review contract defines priorities by effect and gives one example for each:

- P0: data loss, a security compromise, or a crash on a common path (deleting user records).
- P1: wrong behavior under normal use (rejecting valid input).
- P2: an edge case or degraded behavior with a workaround (retrying succeeds).
- P3: style or maintainability (a confusing name).
- Only P0 and P1 findings block.

The contract is included in the common review input sent to pi, Claude, and Codex. The contract test checks all four definitions and the blocking rule; backend integration tests check that the emitted input contains them for each backend.

## Repeatability measurement

Each run used the exact PR head and target from the t172 report, one-pass, pi, `openai/gpt-5.6-luna`, `unslop` and `no-comments`, with a fresh `GREENHOUSE_HOME`. The before binary was built from this Task's parent commit `9b9ba69`. The after binary was built with this Task's contract wording. Findings below use reviewer-reported line ranges. A clean review has no finding location.

### PR #1152

Head `cd3843165060ffbd9115dbe431ce306f8759af15`; target `c642d04ce6f5a2c2749296b2f050a6acf8a23b29`.

| Version | Run | Verdict | Findings (priority, file:line) |
|---|---:|---|---|
| Before | 1 | clean | none |
| Before | 2 | clean | none |
| Before | 3 | clean | none |
| After | 1 | findings | P1 `internal/app/review_cap.go:27-29` |
| After | 2 | findings | P1 `internal/app/review_cap.go:27-29` |
| After | 3 | clean | none |

The same location received P1 in both after-runs that reported it. The before runs reported no location, so priority consistency for this location is not comparable across versions. The clean/finding outcome still varied after the change.

### PR #1158

Head `f352b9ce6c586cc606d0200d5ee97ddeda54f661`; target `a90e7175ae724ff3fef8858627b0069542657bb0`.

| Version | Run | Verdict | Findings (priority, file:line) |
|---|---:|---|---|
| Before | 1 | findings | P1 `internal/app/watch_runtime.go:1096-1104` |
| Before | 2 | findings | P1 `internal/app/service.go:837-849` |
| Before | 3 | findings | P2 `internal/app/watch_runtime.go:1096-1104` |
| After | 1 | findings | P1 `internal/app/service.go:837-849`; P2 `internal/app/watch_runtime.go:1096-1104` |
| After | 2 | clean | none |
| After | 3 | clean | none |

At `internal/app/service.go:837-849`, the repeated finding kept P1 in both versions. At `internal/app/watch_runtime.go:1096-1104`, priority was not stable before (P1 and P2); its after-run finding was P2, matching one before-run and not the other. Both PRs still had clean/finding outcome variation after the change. These six-run samples do not establish that the wording alone makes every defect's priority stable.

## Verification

- Red before implementation: `go test ./internal/engine -run '^TestReviewerContractDefinesPrioritiesAndBlockingGate$' -count=1` failed because all four definitions and the blocking rule were absent.
- Green: the same focused test passed after adding the contract wording.
- Backend prompt delivery: `go test ./internal/engine -run '^(TestOnePassReviewThroughClaudeAndCodex|TestReviewDeliversNumberedCandidateFiles|TestReviewerContractDefinesPrioritiesAndBlockingGate)$' -count=1` passed. Pi, Claude, and Codex emitted review inputs containing the definitions and gate.
- `go vet ./...` passed.
- `go test ./...` passed.
- `go test -race ./...` passed.
- `golangci-lint run ./...` passed with 0 issues.

## Risk and rollback

The examples may not resolve borderline judgments, and the six-run samples still varied. Revert `internal/engine/execution.go` to restore the prior prompt. Revert the assertions and fake-backend input capture to restore the prior tests.
