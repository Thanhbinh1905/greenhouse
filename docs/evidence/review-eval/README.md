# Review evaluation runs

Each directory is one live run of `go run ./eval run` over the whole corpus. See [the README](../../../README.md#review-evaluation) for the command and metric definitions.

## Configuration

All runs used the same setup:

- Reviewer: Greenhouse independent review, backend `pi-json-v1`, Pi 1.5.0, model `openai/gpt-5.6-luna`. Pi reported provider `openai`, model `gpt-5.6-luna`, and thinking level `medium` (the default, because Greenhouse gives Pi an empty agent directory) for every case.
- Endpoint: this machine's Pi configuration routes the `openai` provider to a local proxy at `http://127.0.0.1:8317/v1`. Greenhouse loads no custom provider file, so the eval shim passed this as `-base-url`. The API key came from `pi auth print-api-key --provider openai` through `OPENAI_API_KEY`; no output file contains it.
- Run settings: `-parallel 4`, rules `unslop` and `no-comments`, preset `one-pass`, limit 4.
- Cost is Pi catalog pricing. The proxy may bill differently.

## Runs

| Run | Product change | Corpus | High-severity recall | Recall | Precision | Clean blocking | Inconclusive | Cost | Mean latency | Bar |
|---|---|---|---|---|---|---|---|---|---|---|
| [01-baseline](01-baseline/report.md) | none (product as on `main`) | rev 1 | 0% (0/9) | 0% | n/a | 3 | 16/16 | $0.0251 | 19.9s | not met |
| [02-event-fix](02-event-fix/report.md) | accept Pi system messages | rev 1 | 89% (8/9) | 92% | 93% | 1 | 2/16 | $0.0228 | 18.3s | not met |
| [03-candidate-files](03-candidate-files/report.md) | numbered candidate files, no-checkout contract | rev 1 | 100% (9/9) | 100% | 87% | 1 | 0/16 | $0.0137 | 8.8s | not met |
| [04-corpus-r2](04-corpus-r2/report.md) | same as 03 | rev 2 | 100% (9/9) | 92% | 92% | 0 | 0/16 | $0.0131 | 7.7s | met |
| [05-corpus-r2](05-corpus-r2/report.md) | same as 03, repeated | rev 2 | 100% (9/9) | 100% | 88% | 0 | 0/16 | $0.0136 | 8.5s | met |

Priority accuracy was 100% in every run that produced findings.

## Changes and root causes

**01 to 02: every live review was inconclusive.** Greenhouse logged `malformed JSON event` for all 16 cases although Pi exited 0 and its answers were valid. Pi 1.5.0 emits a `system` `message_start` and `message_end` whose `content` is the string `""`. The event decoder required an array for every message, so the first system event invalidated the stream. Fake-Pi fixtures never emitted that event. The decoder now reads `content` only from the assistant's final message. `TestVerdictAcceptsPiSystemMessageWithStringContent` failed with `malformed JSON event` before the fix.

**02 to 03: missing candidate and wrong line numbers.** Two failures shared one cause: the reviewer could not see the candidate files.

- `clean-directive-and-docs` was inconclusive. The reviewer guidance says to inspect the exact candidate, and Pi has read tools, but its working directory is empty. The model searched for the changed files, found none, and declined to judge.
- `bug-empty-average` named the right defect at line 34 of a 32-line file. Greenhouse rejected the review with `finding range outside changed hunks`, so a detected bug scored as missed. The model had to count lines from hunk headers.

The review bundle now includes `Files`: the candidate content of each changed path, every line prefixed with its line number. The contract says that findings cite these numbers, that no checkout is available, and that `inconclusive` is only for input that cannot be assessed. `TestReviewDeliversNumberedCandidateFiles` covers deleted files, a missing final newline and the contract text. Inconclusive reviews fell to zero and mean latency halved because the model stopped searching an empty directory.

**03 to 04: corpus correction.** In run 03, `clean-directive-and-docs` received a P1 finding: the change adds a `go:generate stringer` directive, but the generated `String` method was not committed although the intent says "Generate String for Level". That finding is correct. The case was not clean, so the ground truth was wrong, not the reviewer. Revision 2 commits the real `stringer` output (golang.org/x/tools v0.38.0) in the head tree and marks its generated-code marker and comments as noise. No other case changed. Runs 04 and 05 use the same product code as 03.

## Remaining weaknesses

These did not affect the bar but are real:

- In 04 and 05 the reviewer flagged the user quotation in `policy-unslop-readme` as an unsupported testimonial (P2). The rule scope says to preserve User quotations; the model reads a customer quote in prose as subject to `unslop`.
- In 03 and 05, `noise-legacy-context` received a speculative finding (P2, then P1) that `InStock` panics for a map entry holding a nil `*Item`. Nothing in the change creates such entries. The same input produced different priorities across runs.
- In 04 one of three narrating comments in `policy-needless-comments` was missed.
- Two runs per configuration are not enough to estimate variance. Expect the counts above to move by a finding or two per run.

## Reproducing

Commit IDs in each `report.json` identify the product code. `input.json` in each case directory holds the exact bytes Greenhouse delivered, so the review input can be replayed without the corpus. Runs 01 to 03 used corpus revision 1 (digest `77d755d3…`); 04 and 05 used revision 2 (digest `4f1ccef6…`).

Stored `events.jsonl` files exclude Pi's `message_update` streaming deltas; the matching `message_end` events carry the same final content.
