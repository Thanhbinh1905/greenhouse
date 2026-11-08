# Review evaluation runs

Each directory is one live run of `go run ./eval run` over the whole corpus. See [the README](../../../README.md#review-evaluation) for the command and metric definitions.

## Configuration

All runs used the same setup:

- Reviewer: Greenhouse independent review, backend `pi-json-v1`, Pi 1.5.0. Runs 01 to 05 used `openai/gpt-5.6-luna`; runs 06 to 09 compare other models. For every case, Pi reported the requested provider and model and thinking level `medium`. Medium is Pi's default, which applies because Greenhouse gives Pi an empty agent directory.
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
| [06-gpt-6.1-sol-r1](06-gpt-6.1-sol-r1/report.md) | same as 03, `openai/gpt-6.1-sol` | rev 2 | 100% (9/9) | 85% | 86% | 0 | 0/16 | $0.1173 | 9.1s | met |
| [07-gpt-6.1-sol-r2](07-gpt-6.1-sol-r2/report.md) | same as 06, repeated | rev 2 | 100% (9/9) | 85% | 86% | 0 | 0/16 | $0.1153 | 8.7s | met |
| [08-gpt-6-luna-r1](08-gpt-6-luna-r1/report.md) | same as 03, `openai/gpt-6-luna` | rev 2 | 100% (9/9) | 92% | 92% | 0 | 0/16 | $0.0046 | 4.6s | met |
| [09-gpt-6-luna-r2](09-gpt-6-luna-r2/report.md) | same as 08, repeated | rev 2 | 89% (8/9) | 85% | 85% | 0 | 0/16 | $0.0045 | 4.2s | met |

Priority accuracy was 100% in every run that produced findings, except 06 (91%).

## Changes and root causes

**01 to 02: every live review was inconclusive.** Greenhouse logged `malformed JSON event` for all 16 cases although Pi exited 0 and its answers were valid. Pi 1.5.0 emits a `system` `message_start` and `message_end` whose `content` is the string `""`. The event decoder required an array for every message, so the first system event invalidated the stream. Fake-Pi fixtures never emitted that event. The decoder now reads `content` only from the assistant's final message. `TestVerdictAcceptsPiSystemMessageWithStringContent` failed with `malformed JSON event` before the fix.

**02 to 03: missing candidate and wrong line numbers.** Two failures shared one cause: the reviewer could not see the candidate files.

- `clean-directive-and-docs` was inconclusive. The reviewer guidance says to inspect the exact candidate, and Pi has read tools, but its working directory is empty. The model searched for the changed files, found none, and declined to judge.
- `bug-empty-average` named the right defect at line 34 of a 32-line file. Greenhouse rejected the review with `finding range outside changed hunks`, so a detected bug scored as missed. The model had to count lines from hunk headers.

The review bundle now includes `Files`: the candidate content of each changed path, every line prefixed with its line number. The contract says that findings cite these numbers, that no checkout is available, and that `inconclusive` is only for input that cannot be assessed. `TestReviewDeliversNumberedCandidateFiles` covers deleted files, a missing final newline and the contract text. Inconclusive reviews fell to zero and mean latency halved because the model stopped searching an empty directory.

**03 to 04: corpus correction.** In run 03, `clean-directive-and-docs` received a P1 finding: the change adds a `go:generate stringer` directive, but the generated `String` method was not committed although the intent says "Generate String for Level". That finding is correct. The case was not clean, so the ground truth was wrong, not the reviewer. Revision 2 commits the real `stringer` output (golang.org/x/tools v0.38.0) in the head tree and marks its generated-code marker and comments as noise. No other case changed. Runs 04 and 05 use the same product code as 03.

## Model comparison

Runs 04 to 09 use the same product code, corpus revision 2, criteria and settings. Each model ran twice; the table totals both runs.

| Model | High-severity recall | Recall | Precision | Priority accuracy | Clean false positives (blocking) | Noise false positives (blocking) | Cost per pass | Mean and max latency |
|---|---|---|---|---|---|---|---|---|
| `openai/gpt-5.6-luna` | 100% (18/18) | 96% (25/26) | 90% (26/29) | 100% | 0 (0) | 1 (1) | $0.0133 | 8.1s / 15.1s |
| `openai/gpt-6.1-sol` | 100% (18/18) | 85% (22/26) | 86% (24/28) | 95% | 2 (0) | 2 (0) | $0.1163 | 8.9s / 15.8s |
| `openai/gpt-6-luna` | 94% (17/18) | 88% (23/26) | 88% (23/26) | 100% | 0 (0) | 2 (2) | $0.0046 | 4.4s / 7.1s |

- `gpt-6.1-sol` found every planted bug but missed two of three narrating comments in both runs and flagged a necessary doc comment on a clean case (P3). It costs about nine times as much as `gpt-5.6-luna` with no measured gain.
- `gpt-6-luna` is the cheapest and fastest. In run 09 it flagged negative page arguments in `bug-pagination-bounds` instead of the unclamped end index, missing a planted bug. Both runs raised the same P1 noise finding.
- Recommendation: use `openai/gpt-5.6-luna` as the reviewer model. It had the best recall and precision, no clean-case false positive, and costs about $0.013 per 16-case pass. Re-measure with this corpus before changing models.

This is not a cross-vendor comparison. The proxy behind the `openai` provider serves only OpenAI models. xAI has OAuth credentials, but its API answered `402 "You have run out of credits or need a Grok subscription."` Anthropic, Google, OpenRouter and Meta have no configured credentials on this machine. The nine recorded runs cost $0.33 at Pi catalog prices.

## Remaining weaknesses

These did not affect the bar but are real:

- In 04 and 05 the reviewer flagged the user quotation in `policy-unslop-readme` as an unsupported testimonial (P2). The rule scope says to preserve User quotations; the model reads a customer quote in prose as subject to `unslop`.
- In 03 and 05, `noise-legacy-context` received a speculative finding (P2, then P1) that `InStock` panics for a map entry holding a nil `*Item`. Nothing in the change creates such entries. The same input produced different priorities across runs.
- In 04 one of three narrating comments in `policy-needless-comments` was missed.
- Two runs per model are not enough to estimate variance. Expect the counts above to move by a finding or two per run.

## Reproducing

Commit IDs in each `report.json` identify the product code. `input.json` in each case directory holds the exact bytes Greenhouse delivered, so the review input can be replayed without the corpus. Runs 01 to 03 used corpus revision 1 (digest `77d755d3…`); 04 and 05 used revision 2 (digest `4f1ccef6…`).

Stored `events.jsonl` files exclude Pi's `message_update` streaming deltas; the matching `message_end` events carry the same final content.
