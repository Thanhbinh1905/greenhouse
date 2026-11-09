# Fixer evaluation runs

Each directory is one live run of `go run ./eval fix-run` over the whole fixer corpus. See [the README](../../../README.md#fixer-evaluation) for the command and metric definitions.

## Configuration

Every run used the following setup, with the thinking change noted below. `report.json` in each directory records it exactly.

- Fixer: Greenhouse one-pass selected-finding repair with the default fixer, Pi 1.5.0 (`pi-json-v1`) and `openai/gpt-5.6-luna`. Pi reported that provider and model for every case, with thinking level `off` in runs 01 to 04 and `medium` in run 05.
- Review: the eval binary stands in for the reviewer and returns each case's seeded findings, so every run repairs the same review. Greenhouse still validates that review and computes the finding IDs.
- Run settings: mode `auto`, preset `one-pass`, limit 4, rules `unslop` and `no-comments`, `-parallel 4`. `fixer.json` checks are `go vet ./...` and `go test ./...` with the absolute Go 1.26.7 executable.
- Endpoint: the `openai` provider routed to the local proxy at `http://127.0.0.1:8317/v1`. The key came from `pi auth print-api-key --provider openai` through `OPENAI_API_KEY`; no output file contains it.
- Cost: Pi reports a cost of 0 because the provider entry Greenhouse writes has no prices. The harness prices the reported tokens with Pi's catalog price for `openai/gpt-5.6-luna`: $0.20 input, $1.20 output, $0.02 cache read and $0.25 cache write per million tokens. The proxy may bill differently.
- Corpus: revision 1, digest `18abdc4e…`, for every run.

## Runs

| Run | Product change | Fixed | Regressions | Scope violations | Correct responses | Cost | Mean latency | Bar |
|---|---|---|---|---|---|---|---|---|
| [01-baseline](01-baseline/report.md) | none (fixer instructions as on `main`) | 80% (8/10) | 0 | 2 | 10/10 | $0.0115 | 12.1s | not met |
| [02-scope-contract](02-scope-contract/report.md) | response contract: unselected findings are out of scope | 90% (9/10) | 0 | 0 | 9/10 | $0.0128 | 12.7s | met |
| [03-scope-contract-repeat](03-scope-contract-repeat/report.md) | same as 02, repeated | 90% (9/10) | 0 | 0 | 9/10 | $0.0129 | 12.2s | met |
| [04-scope-for-fixed](04-scope-for-fixed/report.md) | scope sentence limited to findings answered fixed | 90% (9/10) | 0 | 0 | 9/10 | $0.0115 | 12.3s | met |
| [05-reasoning-and-response-validation](05-reasoning-and-response-validation/report.md) | refuse fixed without edits; preserve catalog reasoning | 90% (9/10) | 0 | 0 | 9/10 | $0.0130 | 14.3s | met |

Run 05 records the current product at `f281495`. Every hidden acceptance test passed in all five runs. The five passes cost $0.062.

## Changes and root causes

**01: the fixer also repaired unselected findings.** `fix-empty-average` and `fix-unlock-missing` each carry a second, unselected finding: a narrating comment and a promotional doc comment. In both cases the fixer fixed the selected defect correctly and also deleted the unselected comment. Its responses said so: "also removed the redundant loop comment in the changed hunk". The input gave the fixer the selected findings and the rule criteria with their repair guidance, but nothing said that other violations were out of scope. Greenhouse's path check cannot catch this, because the edit stays inside an authorized file.

The response contract, delivered to every selected-finding repair in one-pass and full pipelines, now says: "Unselected findings are out of scope: leave every other line as it is, even one that seems to break a criterion or hold another defect. Criteria apply to the lines you change." `TestFixerInputConfinesEditsToSelectedFindings` failed before the change because the delivered contract lacked it. Both scope violations disappeared in 02, 03 and 04, and no case that was fixed in 01 regressed.

**02 to 04: the disputed case answers fixed.** `dispute-guarded-division` selects a false finding: a division by zero that the guard above it already prevents. In 01 the fixer disputed it. In 02 and 03 it answered `fixed` without editing, with the reason "Rate now returns 0 when requests is zero", a change it had not made. The 02 sentence "Change only the lines that resolving the Selected findings requires" presumes a fix, so 04 limits edits to findings answered fixed instead. Run 04 still answered `fixed`, this time with the reason "Rate already guards against requests == 0", which reads `fixed` as "the code is correct". With one dispute case and four runs this cannot separate a wording effect from variance. Those versions recorded these repairs as `no-change` and committed nothing.

**05: contradictions fail, and thinking is restored.** Greenhouse now refuses a `fixed` response unless the candidate changes that finding's file. The dispute case again answered `fixed` without editing. It now ends as `fixer-failed`, with no repair commit and a log naming the unchanged file. The response remains in the record as evidence of the failure. The other nine cases pass without regressions or scope violations.

Greenhouse also carries Pi's trusted catalog reasoning flag into its private provider entry. Pi reported `medium` for every case. Executions and attempts record the effective level from Pi's final assistant message. The [review rerun](../review-eval/12-reasoning-restored/report.md) confirms no regression.

The instructions name no case, file or defect from the corpus.

## Remaining weaknesses

- The corpus has one dispute case, so resolution-response accuracy rests on a single case per run.
- Scope is line-based: an edit must change lines inside the case's scope ranges, and an insertion must border one. A wrong edit inside the scope range shows up only through the hidden tests.

## Reproducing

```sh
go run ./eval fix-check
OPENAI_API_KEY=... go run ./eval fix-run -base-url http://127.0.0.1:8317/v1 -parallel 4 \
  -label "what changed" -out docs/evidence/fixer-eval/NN-name
```

`input.json` in each case directory holds the exact bytes Greenhouse delivered to the fixer, and `argv.json` the exact arguments. Stored `events.jsonl` files exclude Pi's `message_update` streaming deltas.
