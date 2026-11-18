# Review budget and verdict recovery

Issue: #27. Implementation: `f1d807a8b01932622c1d0a2e3d43f03972fb76f3`.

The controller defaults to a three-minute wall-time budget, reserves its last 20% for one finalization turn, and sends numbered changed-hunk excerpts with 20 context lines. Invalid or missing output gets the same recovery turn. Findings outside changed hunks are dropped individually with diagnostics. Attempt usage includes both turns.

## Method

Runs used the existing `go run ./cmd/eval run` harness, native Pi 1.5.0, `openai/gpt-5.6-luna`, and Go 1.26.7. Corpus runs used `-parallel 4`; the real diff used `-parallel 1`. Baseline source was `95082ada320b7ad65feb5cd7aac7c3077755588d`. The baseline archive was committed as a scratch snapshot so the harness could record a version. The corpus after-run used the working tree with hunk excerpts and low-effort finalization, before subsequent native-session configuration fixes; the real repeat used the implementation commit above.

Reported tokens are input + output + cache-read + cache-write tokens, preserving Pi's accounting. Reasoning tokens are already part of output. These are backend-reported totals, not a hard token cap. Killed requests can omit usage, so totals can undercount interrupted work. These runs measure cost and completion; findings were not independently adjudicated.

Raw inputs, events, timing, and available reports remain in the Posse task scratch directory `/home/thanhbinh/.posse/scratch/greenhouse/t142/`. The accompanying JSON retains per-case metrics without copying model transcripts into the repository.

## Real diff

The fixture contains the eight changed files from commit `ba3ac3da6e2d6def8f77e2be3c819f091e5bbd5a` and its first parent: 493 additions and 18 deletions, 511 changed lines. File contents are unmodified; paths are flattened from `internal/engine/`. Unchanged repository files are absent. Both runs used this identical fixture and neutral criteria.

| Run | Input | Output | Cache read | Total tokens | Review time | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Before | 541,359 | 27,350 | 0 | 568,709 | 508.3 s | Inconclusive: invalid identity or coverage |
| After, final commit | 126,538 | 6,650 | 111,360 | 244,548 | 156.8 s | Findings |

Reported tokens fell 57.0%; the admitted input shrank from 273,053 to 98,951 bytes. The after-run stopped exploration at the reserved boundary and recovered a verdict in the same session. Its finding has not been adjudicated. An earlier after-run returned clean using 234,545 tokens; this variation is not evidence of accuracy. That earlier run exposed a timing-capture bug that counted only finalization. The final repeat includes both turns.

Commands used `-corpus <scratch>/real-corpus -label t142-real-before -out <scratch>/real-before` on the baseline, and `-label t142-real-final -out <scratch>/real-final` on the implementation. The fixture metadata records the source commit and derivation.

## Reproduction and backend checks

Before implementation, CLI tests failed with `finding range outside changed hunks` and `missing settled final answer`. The same tests now produce verdicts. Additional CLI tests cover malformed output, exactly one retry, a hung recovery turn, a forced budget boundary, an unrelated-file finding, all findings dropped, native Claude/Codex resume arguments, and aggregate attempt usage exposed by AXI inspect.

Native live checks also completed through Claude and Codex. Claude returned a findings verdict. Codex returned clean; a second run replaced only the first final message with invalid text and then resumed the native thread to a clean verdict. AXI inspect recorded `finalization: true` and 46,083 tokens for that attempt. These smoke checks establish native-session compatibility, not review accuracy.

The eval shim now appends both turns' events and records the start before launching the child. Its regression tests first reproduced lost exploration tokens and lost interrupted-turn time, then passed after the fixes.

## Eval corpus

Both runs used all 166 cases from corpus revision 5. Commands used `-label t142-before -out <scratch>/eval-before` and `-label t142-final-hunk-context -out <scratch>/eval-final`.

| Metric | Before | After |
| --- | ---: | ---: |
| Input tokens | 5,470,279 | 1,711,228 |
| Output tokens | 591,630 | 253,724 |
| Cache-read tokens | 9,629,056 | 1,110,400 |
| Total reported tokens | 15,690,965 | 3,075,352 |
| Conclusive reviews | 155 | 163 |
| Inconclusive reviews | 7 | 0 |
| Admission failures | 3 | 3 |
| Harness settle timeouts | 1 | 0 |
| Finalization turns | 0 | 15 |

Reported tokens fell 80.4%. All 15 after-run finalization turns produced verdicts. A conclusive verdict can still miss defects or report false positives. These counts do not establish review accuracy.

Both runs rejected `local-large-authority-decisions`, `local-large-independent-reviews`, and `local-large-pipeline-gates` before review because their inputs exceeded the admission bound. The baseline `public-clean-werkzeug-d441b0c` exceeded the harness's 12-minute settle limit. The baseline harness left that reviewer running after its timeout and exit. A final process scan found the supervisor and stopped its tree with SIGTERM. Its final captured usage, including work after the harness timeout, remains in the total even though the controller never recorded a completed attempt. This cleanup leak inflates the baseline cost; the comparison includes the cost actually reported before cleanup. Excluding this case from both runs gives 8,099,065 tokens before and 3,018,956 after, a 62.7% reduction across the other 165 cases.

The harness automatically judged findings after review. Its report writer then failed with `json: error calling MarshalJSON for type json.RawMessage: unexpected end of JSON input`. An earlier intermediate run failed the same way. A likely cause is an empty raw JSON field in an admission-error record. This is a harness issue, separate from reviewer output. Per-case final statuses and usage were preserved directly from controller state before harness cleanup; event usage supplies cache accounting and the timed-out baseline's usage. The accompanying [metrics JSON](t142-review-budget.json) records both. Judge tokens are excluded. Raw timing remains incomplete for the timed-out baseline, so no whole-corpus latency claim is made.

## Verification

- `go vet ./...`: passed.
- `GOMAXPROCS=4 go test ./... -timeout 20m`: passed; engine 938.974 s.
- `GOMAXPROCS=4 go test -race ./... -timeout 20m`: passed; engine 878.732 s.
- `git diff --check`: passed.

Earlier full-suite runs hit Go's default ten-minute timeout under concurrent machine load. The retries above completed without changing test behavior. After the suites finished, 37 task-owned fixture daemons remained, including leftovers from timed-out runs. They received SIGTERM. Evaluation daemons were excluded. This cleanup leak is tracked separately in #23.

## Review follow-up

Merged `origin/main` at `aab2718` before addressing the relayed findings from run `cfde1de7`.

The private-session symlink finding was real. With Landlock disabled, both native backend CLI fixtures changed their original credential files through session symlinks. The test failed with `native credentials modified through private session: "modified"`. Native credentials and settings now use independent private copies with mode `0400`. Claude onboarding state already used a private writable copy. The test deliberately changes the copy's permissions and contents, then verifies that the original remains unchanged. Both backends complete their reviews. This prevents write-through access; degraded confinement still permits direct access to other filesystem paths.

The shared-deadline finding is disputed. `runBoundedReview` already calls `runner.ProcessContext` before obtaining the deadline and deriving exploration's 80% share. Go's child contexts preserve the earlier parent deadline, so later `RunProcess` calls cannot restart the executor timeout. The added CLI regression uses `executor_timeout: 2s`, invalid exploration output, and a hung finalization turn. The original implementation passes with `reason: review_budget`. Removing that existing `ProcessContext` call in an isolated scratch control makes the test fail with `reason=crash finalization=true`. No deadline implementation change was needed.

The targeted tests pass. A native Claude review returned clean in 10.3 seconds. A native Codex run also resumed its original thread with the copied credentials and returned clean, recording `finalization: true` and 32,618 tokens. Original corpus measurements remain unchanged because this follow-up changes session file isolation rather than review prompts or budgets.

Follow-up checks: `go vet ./...` passed. `GOMAXPROCS=4 go test ./... -timeout 20m` passed (engine 581.529 s), as did `GOMAXPROCS=4 go test -race ./... -timeout 20m` (engine 636.535 s). Lint against the follow-up base, `golangci-lint run --new-from-rev=6e7ee6b ./...`, reported zero issues. Full-repository lint reported pre-existing failures outside this follow-up; its log remains in task scratch as `followup-lint.log`.

## Durable-state compatibility

The pre-upgrade shared-state snapshot reproduced `invalid durable review evidence` in a fresh private home. The changed reviewer prompt made stored arguments differ from the loader's current expected arguments. The failure was not caused by a missing budget value.

The regression fixture `internal/engine/testdata/main-29a6f29-review-state.json` was generated by main commit `29a6f29` through its existing CLI fixture: prepare a synthetic one-file review, install the fake Pi backend, review, wait, stop the daemon, and copy its state. It contains no shared-state content. `TestMainReviewStateLoadsWithoutBudgetFields` failed with `main review state rejected: invalid durable review evidence` before the fix.

Records without `review_budget` now accept the exact historical prompt argument shape as well as the current shape. Budgeted reviews still require the current arguments. Loading preserves stored evidence. Tests reject an added tool argument and reject historical arguments on a budgeted review. The generated-state regression and existing legacy-state regression pass. A rebuilt binary loaded the private shared-state snapshot through AXI with `outcome: ok`; that private daemon was stopped afterward. The private snapshot and AXI output remain outside the repository.

Compatibility checks: `go vet ./...` passed. `GOMAXPROCS=4 go test ./... -timeout 20m` passed (engine 373.456 s), and `GOMAXPROCS=4 go test -race ./... -timeout 20m` passed (engine 411.500 s). `golangci-lint run --new-from-rev=e5ee01e ./...` reported zero issues.

## Claude home isolation

Review round 3 found that redirecting `CLAUDE_CONFIG_DIR` left `HOME` pointing at the original home. The new CLI regression disables Landlock, gives the original home and private config distinct onboarding contents, then reads and writes `HOME/.claude.json` from the reviewer. Before the fix it failed with `reviewer read original home onboarding instead of private copy: "original onboarding"`.

Claude now receives `HOME` pointing at the private session directory, alongside `CLAUDE_CONFIG_DIR`. The regression reads the staged private content and updates that copy while leaving the original unchanged. Existing config-write probes still target the original paths explicitly. Targeted session and recovery tests pass. Native Claude completed a review in 13.1 seconds using the private home. Merged `origin/main` through `ed5d0d4`, then through `d73fb9f` after PR #31 landed during verification.

Checks after merging `ed5d0d4`: `go vet ./...` passed. `GOMAXPROCS=4 go test ./... -timeout 20m` passed (engine 799.522 s), as did `GOMAXPROCS=4 go test -race ./... -timeout 20m` (engine 829.524 s). Lint against `f9a7969` reported zero issues before the merges.

After merging `d73fb9f`, vet and the full normal suite passed (engine 326.501 s). The first full race run failed `TestExecutableSelectedRepair` after its SIGKILL restart: the repair evidence remained, but the run reverted to the earlier review status. Five isolated race repetitions passed. A suspected cause is inspect exposing completed in-memory state while `commitWhileServing` is still persisting it; the test immediately kills the daemon. This separate commit-visibility issue was reported to the Lead. The failing log remains in task scratch as `claude-home-final-race.log`.

The full race retry passed (engine 346.010 s), with no code changes between the failed run and retry. Final checks after merging `d73fb9f`: vet, normal tests, race tests, and `git diff --check` passed.

## Deletion hunk anchors

Review round 4 found that zero-length candidate hunks reused old-file ranges. With Git `diff.context=0`, the CLI regression deletes old lines 21-80, then returns findings at candidate lines 20 and 60. Before the fix, Greenhouse dropped the deletion anchor at line 20 and kept the unrelated finding at line 60. The range regression reported `map[sample:[[21 80]]]`, and the excerpt contained 100 candidate lines instead of the expected 40.

Pure deletions in surviving files now use the candidate `+start` anchor, with a minimum of line 1. Whole-file deletions keep the old range. The CLI regression keeps the line-20 finding, drops line 60 with a diagnostic, and captures the expected candidate excerpt ending at line 40. Range tests cover deletions at the beginning, middle, and end, plus whole-file deletion. The targeted CLI and range tests pass.
