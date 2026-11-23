# t198: verdicts at the review budget

## Result

A real CLI review of Posse PR 1167 on the pre-change Greenhouse `main` ended `inconclusive` with `reason: review_budget`. Its finalization process exited with code 255 and returned no usable verdict. The transcript shows why: exploration used the first 80% of the 3-minute budget, accumulated more than 2.3 MB of agent output, and made several long tool-use turns. The finalization turn inherited that large session and had about 36 seconds left, which was not enough to return a structured verdict.

The controller now stops exploration at the halfway point, giving the finalization turn the other half of the budget. At expiry it retains a complete clean or findings verdict, converts a verdict with uncovered paths to partial, and removes the verdict when there is no usable result. Both finalization and non-finalization outcomes use this settlement rule. Finalization input also omits paths excluded before review so the agent cannot claim coverage for paths that the controller will reject.

Three post-fix real CLI runs on PR 1167 settled as partial within the default 3-minute budget. Each lists every uncovered path and its reason below. The result remains partial because the large baseline migration exceeds hunk-window limits and the prompt exceeds the model context limit for additional files.

## Reproduction and timings

The run used a private `GREENHOUSE_HOME`, an isolated checkout at PR head `3c98dad21e3a12e8f75b811c3e085ec8ae8d910d`, target `5a6f32bf8bdf16624af0d59da5f9f09194db3f90`, the Greenhouse CLI, reviewer pi with its default model, and the default 3-minute review budget.

| Build | Status | Finalization | Exploration stdout | Finalization stdout | Tokens | Cost (USD) | Wall (s) |
|---|---|---:|---:|---:|---:|---:|---:|
| Before fix (`2f51671`) | inconclusive (`review_budget`) | yes | 1,599,301 bytes | 732,481 bytes, no verdict | 500,626 | 0.05950 | 185.1 |

The transcript records tool-use turns lasting 43.6s and 31.9s near the end of exploration. The output includes repeated large file excerpts; the finalization session inherited this history and produced no completed verdict before its deadline. Both executor records exited 255. This is not the earlier assumption that the first pass returned partial data too late: the reviewer spent its available time exploring and the final turn itself ran out of time.

## Post-fix runs

| Run | Verdict / status | Uncovered | Tokens | Cost (USD) | Wall (s) |
|---|---|---:|---:|---:|---:|
| 1 | partial / partial-review | 26 | 573,584 | 0.07674 | 132.9 |
| 2 | partial / partial-review | 23 | 429,648 | 0.06076 | 152.5 |
| 3 | partial / partial-review | 26 | 405,784 | 0.06884 | 137.4 |

Run 1 uncovered paths:

| Path | Reason |
|---|---|
| `internal/app/update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_upgrade_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/baseline_migration_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/greenhouse_verdicts_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/legacy_migration_checksums.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_baseline_fixture_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00050_validation_state.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00055_functional_vocabulary.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00069_project_history_revisions.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/20261120180000_baseline.sql` | hunk windows exceed 65536 bytes |
| `internal/store/schema_guard.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/schema_guard_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/task_pane_shell_migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/testdata/v033.0-schema.json` | hunk windows exceed 65536 bytes |
| `internal/store/upgrades/00098_greenhouse_request_resume.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/upgrades/00099_provider_findings.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/vocabulary_migration_compat.go` | not covered by reviewer within budget |
| `internal/store/vocabulary_migration_compat_test.go` | not covered by reviewer within budget |
| `internal/store/vocabulary_review_test.go` | not covered by reviewer within budget |
| `scripts/e2e-durations.txt` | prompt exceeds model context (141350 tokens > 64000) |
| `scripts/stress-changed-tests.sh` | prompt exceeds model context (141350 tokens > 64000) |

Run 2 uncovered paths:

| Path | Reason |
|---|---|
| `internal/app/update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_upgrade_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/baseline_migration_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/greenhouse_verdicts_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/legacy_migration_checksums.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_baseline_fixture_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00050_validation_state.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00055_functional_vocabulary.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00069_project_history_revisions.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/20261120180000_baseline.sql` | hunk windows exceed 65536 bytes |
| `internal/store/schema_guard.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/schema_guard_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/task_pane_shell_migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/testdata/v033.0-schema.json` | hunk windows exceed 65536 bytes |
| `internal/store/upgrades/00098_greenhouse_request_resume.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/upgrades/00099_provider_findings.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `scripts/e2e-durations.txt` | prompt exceeds model context (141350 tokens > 64000) |
| `scripts/stress-changed-tests.sh` | prompt exceeds model context (141350 tokens > 64000) |

Run 3 uncovered paths:

| Path | Reason |
|---|---|
| `internal/app/update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_update_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/e2e/vocabulary_upgrade_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/baseline_migration_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/greenhouse_verdicts_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/legacy_migration_checksums.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_baseline_fixture_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00050_validation_state.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00055_functional_vocabulary.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/00069_project_history_revisions.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/migrations/20261120180000_baseline.sql` | hunk windows exceed 65536 bytes |
| `internal/store/schema_guard.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/schema_guard_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/store_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/task_pane_shell_migration_compat_test.go` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/testdata/v033.0-schema.json` | hunk windows exceed 65536 bytes |
| `internal/store/upgrades/00098_greenhouse_request_resume.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/upgrades/00099_provider_findings.sql` | prompt exceeds model context (141350 tokens > 64000) |
| `internal/store/vocabulary_migration_compat.go` | not covered by reviewer within budget |
| `internal/store/vocabulary_migration_compat_test.go` | not covered by reviewer within budget |
| `internal/store/vocabulary_review_test.go` | not covered by reviewer within budget |
| `scripts/e2e-durations.txt` | prompt exceeds model context (141350 tokens > 64000) |
| `scripts/stress-changed-tests.sh` | prompt exceeds model context (141350 tokens > 64000) |

## Issue 87

Budget settlement no longer keeps a clean or findings result while changing its status to inconclusive. Complete results settle to their matching terminal status. Results with uncovered paths settle as `partial-review` / `partial`; no-result budget expiry settles as `inconclusive` and clears the result, so `host_result.verdict` is empty. Tests assert these mappings.

## Verification

- Red before fix: `go test ./internal/engine -run '^TestReviewFinalizationGetsHalfOfReviewBudget$' -count=1 -v` failed with `status=inconclusive finalization=true` because the fake finalization response needed 2.5s but the 10s review budget reserved only 2s.
- Green after fix: the same test passes with the half-budget finalization reserve.
- Real CLI reproduction before fix: PR 1167 returned inconclusive / review_budget in 185.1s with 500,626 tokens; both executor turns exited 255 and no verdict was recorded.
- Real CLI post-fix: the three runs above each returned partial within 153s.
- `go test ./...` passed.
- `go vet ./...` passed.
- `golangci-lint run ./...` passed with 0 issues.
- `go test -race ./...` passed.

## Risk and rollback

Reserving half the review budget reduces exploration time for small or fast reviews but gives large reviews time to finalize. Verdict normalization only changes outcomes at budget expiry; incomplete results remain non-clean. Roll back with `git revert bd4eb9fcf6b1e7e2a12df2f2ce6dce8913e345a6` if the larger finalization reserve causes unacceptable loss of review coverage or budget settlement produces a mismatched status.
