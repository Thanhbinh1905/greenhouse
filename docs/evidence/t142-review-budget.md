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
| Input tokens | 5,310,053 | 1,711,228 |
| Output tokens | 585,323 | 253,724 |
| Cache-read tokens | 8,942,336 | 1,110,400 |
| Total reported tokens | 14,837,712 | 3,075,352 |
| Conclusive reviews | 155 | 163 |
| Inconclusive reviews | 7 | 0 |
| Admission failures | 3 | 3 |
| Harness settle timeouts | 1 | 0 |
| Finalization turns | 0 | 15 |

Reported tokens fell 79.3%. All 15 after-run finalization turns produced verdicts. A conclusive verdict can still miss defects or report false positives. These counts do not establish review accuracy.

Both runs rejected `local-large-authority-decisions`, `local-large-independent-reviews`, and `local-large-pipeline-gates` before review because their inputs exceeded the admission bound. The baseline `public-clean-werkzeug-d441b0c` exceeded the harness's 12-minute settle limit. Its captured usage remains in the total even though the controller never recorded a completed attempt.

The harness automatically judged findings after review. Its report writer then failed with `json: error calling MarshalJSON for type json.RawMessage: unexpected end of JSON input`. An earlier intermediate run failed the same way. A likely cause is an empty raw JSON field in an admission-error record. This is a harness issue, separate from reviewer output. Per-case final statuses and usage were preserved directly from controller state before harness cleanup; event usage supplies cache accounting and the timed-out baseline's usage. The accompanying [metrics JSON](t142-review-budget.json) records both. Judge tokens are excluded. Raw timing remains incomplete for the timed-out baseline, so no whole-corpus latency claim is made.

## Verification

- `go vet ./...`: passed.
- `GOMAXPROCS=4 go test ./... -timeout 20m`: passed; engine 938.974 s.
- `GOMAXPROCS=4 go test -race ./... -timeout 20m`: passed; engine 878.732 s.
- `git diff --check`: passed.

Earlier full-suite runs hit Go's default ten-minute timeout under concurrent machine load. The retries above completed without changing test behavior. After the suites finished, 37 task-owned fixture daemons remained, including leftovers from timed-out runs. They received SIGTERM. Evaluation daemons were excluded. This cleanup leak is tracked separately in #23.
