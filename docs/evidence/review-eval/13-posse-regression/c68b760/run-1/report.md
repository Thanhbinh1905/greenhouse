# Review evaluation: posse regression, build c68b760, pass 1

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.2
- Greenhouse: `c68b76030908dcd504e34303774acd5db07b4809`, corpus v1 revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Split: `all`; counts by split: map[holdout:33]; counts by stratum: map[real-posse-review:33]
- Scored with corpus revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Started 2026-11-23T02:12:18Z, finished 2026-11-23T02:36:10Z, parallel 3

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 10% (2/21) |
| Recall | 11% (3/27) |
| Precision | 21% (3/14) |
| Allowed judgment findings (excluded from precision) | 0 |
| Priority accuracy | 67% (2/3) |
| False positives (clean / noise / all) | 2 / 0 / 11 |
| False positives judged a different defect at an expected location | 5 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 3 |
| Inconclusive reviews | 4/33 |
| Cost (Pi catalog pricing) | $1.3854 |
| Latency total / mean / max | 2922.4s / 88.6s / 169.6s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| posse-pr-1131-r1 | outstanding-findings | 0/1 | 1 | 0 | 0/0 | $0.0986 | 165.4s |
| posse-pr-1131-r2 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.1013 | 166.8s |
| posse-pr-1146-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0178 | 66.8s |
| posse-pr-1146-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0169 | 79.9s |
| posse-pr-1152-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0612 | 146.2s |
| posse-pr-1152-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0376 | 68.8s |
| posse-pr-1156-r1 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0397 | 68.5s |
| posse-pr-1156-r2 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0502 | 62.2s |
| posse-pr-1158-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0554 | 86.3s |
| posse-pr-1158-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0533 | 105.5s |
| posse-pr-1159-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0233 | 80.3s |
| posse-pr-1159-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0362 | 61.1s |
| posse-pr-1164-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0115 | 32.4s |
| posse-pr-1164-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0123 | 28.4s |
| posse-pr-1165-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0241 | 156.8s |
| posse-pr-1165-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0263 | 157.2s |
| posse-pr-1167-r1 | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0513 | 0.0s |
| posse-pr-1167-r2 | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0507 | 0.0s |
| posse-pr-1168-r1 | outstanding-findings | 1/2 | 0 | 0 | 1/1 | $0.0375 | 56.2s |
| posse-pr-1168-r2 | outstanding-findings | 0/2 | 2 | 2 | 0/0 | $0.0332 | 80.2s |
| posse-t1222-30c14997 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0419 | 77.3s |
| posse-t1222-41abce87 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0160 | 44.7s |
| posse-t1222-4210ed17 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0171 | 43.9s |
| posse-t1222-9de124d7 | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0150 | 51.8s |
| posse-t1222-a5e704a0 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0266 | 68.8s |
| posse-t1222-bd2af87f | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0637 | 139.8s |
| posse-t1222-c6403c40 | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0656 | 163.0s |
| posse-t1222-c8af1296 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0229 | 57.4s |
| posse-t1222-e7d4350a | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0322 | 84.6s |
| posse-t1222-e86dccac | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0872 | 169.6s |
| posse-t1222-eb193df8 | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0147 | 31.4s |
| posse-task-1241-r1 | outstanding-findings | 0/3 | 1 | 1 | 0/0 | $0.0868 | 158.0s |
| posse-task-1241-r2 | outstanding-findings | 1/3 | 0 | 0 | 0/1 | $0.0574 | 162.9s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
