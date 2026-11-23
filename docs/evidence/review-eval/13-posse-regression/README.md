# Posse PR regression, two builds

The external corpus pins 33 posse cases: the t167 set (ten PRs plus t1241) and eleven t1222 heads whose Greenhouse findings t1236 adjudicated TRUE. Corpus revision 9 expects 27 defects per pass. Each build ran two passes through the real `greenhouse axi` CLI with Pi reviewer `openai/gpt-5.6-luna`, scored by the Pi judge on the defect each finding describes.

- Builds: `ed45ead` (shared daemon at the time of the run) and `c68b760` (main). Each pass ran `cmd/eval` from a worktree at that commit; the evaluator binary was identical.
- Layout: `ed45ead/run-1`, `ed45ead/run-2`, `c68b760/run-1`, `c68b760/run-2`, each with `report.md`, `report.json` and `adjudications.json`. The raw per-case files (`input.json`, `events.jsonl`) were not committed because of their size (213 MB); rescoring uses `adjudications.json`.
- PRs 1131, 1146, 1152, 1156, 1158, 1159, 1164, 1165, 1167, 1168 and t1241 run twice per pass (`-r1`, `-r2`), so each of their defects has 4 opportunities per build. The t1222 cases have 2.

## Recall

| Build | Detected | High-severity detected | Cost, two passes |
|---|---|---|---|
| ed45ead | 6 of 54 (pass 1: 2, pass 2: 4) | 4 of 42 | $4.12 |
| c68b760 | 5 of 54 (pass 1: 3, pass 2: 2) | 4 of 42 | $2.80 |

c68b760 recall is one defect below ed45ead on these cases, so the Posse Lead's condition (c68b760 at least ed45ead) is not met. The gap is within run-to-run noise: both builds detect a handful of the 27 defects, and the three t1222 defects ed45ead found once each (`skill_uses.go`, `validation_candidate_test.go`, `tasks.go`) were missed on c68b760, of which `tasks.go` was an inconclusive review in pass 1.

## Per-case detection

Detected over opportunities, summed across both passes.

| Case | Defect | Expected location | ed45ead | c68b760 |
|---|---|---|---|---|
| posse-pr-1131 | brief-plan-externalization | P1 `plan_blobs.go:94-94` | 0/4 | 0/4 |
| posse-pr-1156 | watch-adapter-environment | P1 `greenhouse_watch.go:159-159` | 0/4 | 0/4 |
| posse-pr-1168 | validation-summary-accepted-without-quote | P1 `validation_summary.go:101-106` | 0/4 | 0/4 |
| posse-pr-1168 | validation-summary-latest-disposition | P1 `validation_summary.go:72-93` | 0/4 | 1/4 |
| posse-t1222-30c14997 | t1222-8a3be43e | P1 `protocol.go:155-156` | 0/2 | 0/2 |
| posse-t1222-41abce87 | t1222-16d9437c | P1 `skill_uses.go:72-74` | 1/2 | 0/2 |
| posse-t1222-4210ed17 | t1222-12568e37 | P2 `validation_candidate_test.go:205-243` | 1/2 | 0/2 |
| posse-t1222-9de124d7 | t1222-4d90ec70 | P1 `tasks.go:1509-1510` | 1/2 | 0/2 |
| posse-t1222-a5e704a0 | t1222-48185cfc | P1 `herdr.go:158-161` | 0/2 | 0/2 |
| posse-t1222-bd2af87f | t1222-005930ee | P1 `review_rounds.go:924-955` | 0/2 | 0/2 |
| posse-t1222-c6403c40 | t1222-57c05255 | P1 `review_rounds.go:147-153` | 0/2 | 0/2 |
| posse-t1222-c6403c40 | t1222-db47db6a | P2 `greenhouse.go:253-267` | 0/2 | 0/2 |
| posse-t1222-c8af1296 | t1222-f91a0722 | P1 `review_verdicts.go:212-212` | 0/2 | 0/2 |
| posse-t1222-e7d4350a | t1222-29b911a0 | P1 `task_plan.go:130-130` | 2/2 | 2/2 |
| posse-t1222-e86dccac | t1222-03812dd3 | P1 `review_provider.go:16-31` | 0/2 | 0/2 |
| posse-t1222-e86dccac | t1222-fbe4c479 | P1 `review_provider.go:102-115` | 0/2 | 1/2 |
| posse-t1222-eb193df8 | t1222-61540fd2 | P1 `merge.go:539-551` | 0/2 | 0/2 |
| posse-task-1241 | cascade-ignores-state-race | P1 `task_dependants.go:137-143` | 0/4 | 0/4 |
| posse-task-1241 | decision-to-untrimmed | P2 `decisions.go:411-435` | 0/4 | 0/4 |
| posse-task-1241 | transitive-dependants-not-cascaded | P2 `task_dependants.go:137-145` | 1/4 | 1/4 |

## Clean cases: blocking results

| Clean case | ed45ead blocking | c68b760 blocking |
|---|---|---|
| posse-pr-1146 | 0/4 | 0/4 |
| posse-pr-1152 | 1/4 | 1/4 |
| posse-pr-1158 | 2/4 | 3/4 |
| posse-pr-1159 | 2/4 | 2/4 |
| posse-pr-1164 | 0/4 | 1/4 |
| posse-pr-1165 | 0/4 | 0/4 |
| posse-pr-1167 | 0/0 | 4/4 |


ed45ead refuses posse 1167 as "review input too large" (`eval-error`, no review, excluded from its summary); c68b760 reviews it and returns inconclusive on all four runs. c68b760 reports 4 inconclusive reviews per pass against 1 for ed45ead on the reviewed cases.

## Expected-defect notes

- PR 1146: no review reported the false nested-migration P1 on `preflight.sh`.
- t1241: no review reported a P0-P2 at `internal/taskplan/plan.go:891-899`.
- The original three t167 misses (1131, 1156, 1168 at `validation_summary.go:101-106`) are missed by both builds. c68b760 matched the other 1168 defect once.
- All heads and targets fetched from GitHub; none was missing.
