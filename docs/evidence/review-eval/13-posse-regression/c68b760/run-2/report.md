# Review evaluation: posse regression, build c68b760, pass 2

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.2
- Greenhouse: `c68b76030908dcd504e34303774acd5db07b4809`, corpus v1 revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Split: `all`; counts by split: map[holdout:33]; counts by stratum: map[real-posse-review:33]
- Scored with corpus revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Started 2026-11-23T02:12:18Z, finished 2026-11-23T02:36:27Z, parallel 3

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 10% (2/21) |
| Recall | 7% (2/27) |
| Precision | 13% (2/15) |
| Allowed judgment findings (excluded from precision) | 0 |
| Priority accuracy | 100% (2/2) |
| False positives (clean / noise / all) | 6 / 0 / 13 |
| False positives judged a different defect at an expected location | 2 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 8 |
| Inconclusive reviews | 4/33 |
| Cost (Pi catalog pricing) | $1.4118 |
| Latency total / mean / max | 2833.9s / 85.9s / 177.9s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| posse-pr-1131-r1 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0841 | 163.9s |
| posse-pr-1131-r2 | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0457 | 0.0s |
| posse-pr-1146-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0136 | 149.9s |
| posse-pr-1146-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0156 | 53.5s |
| posse-pr-1152-r1 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0708 | 158.6s |
| posse-pr-1152-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0350 | 70.0s |
| posse-pr-1156-r1 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0586 | 93.9s |
| posse-pr-1156-r2 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0511 | 64.5s |
| posse-pr-1158-r1 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0546 | 88.6s |
| posse-pr-1158-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0357 | 56.3s |
| posse-pr-1159-r1 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0292 | 82.5s |
| posse-pr-1159-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0455 | 125.1s |
| posse-pr-1164-r1 | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0164 | 32.9s |
| posse-pr-1164-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0238 | 47.3s |
| posse-pr-1165-r1 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0312 | 156.5s |
| posse-pr-1165-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0312 | 152.0s |
| posse-pr-1167-r1 | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0688 | 0.0s |
| posse-pr-1167-r2 | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0535 | 0.0s |
| posse-pr-1168-r1 | clean-review | 0/2 | 0 | 0 | 0/0 | $0.0405 | 81.6s |
| posse-pr-1168-r2 | clean-review | 0/2 | 0 | 0 | 0/0 | $0.0397 | 85.7s |
| posse-t1222-30c14997 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0487 | 119.0s |
| posse-t1222-41abce87 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0162 | 56.7s |
| posse-t1222-4210ed17 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0122 | 38.1s |
| posse-t1222-9de124d7 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0091 | 29.5s |
| posse-t1222-a5e704a0 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0323 | 60.7s |
| posse-t1222-bd2af87f | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0582 | 156.3s |
| posse-t1222-c6403c40 | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0890 | 147.4s |
| posse-t1222-c8af1296 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0222 | 57.4s |
| posse-t1222-e7d4350a | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0516 | 86.8s |
| posse-t1222-e86dccac | outstanding-findings | 1/2 | 2 | 2 | 1/1 | $0.0854 | 177.9s |
| posse-t1222-eb193df8 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0137 | 31.1s |
| posse-task-1241-r1 | outstanding-findings | 0/3 | 1 | 1 | 0/0 | $0.0525 | 104.6s |
| posse-task-1241-r2 | clean-review | 0/3 | 0 | 0 | 0/0 | $0.0761 | 105.7s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
