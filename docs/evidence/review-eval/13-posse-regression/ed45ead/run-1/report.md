# Review evaluation: posse regression, build ed45ead, pass 1

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.2
- Greenhouse: `ed45ead399a81d95fa964b7ed3a35a25ef4b3c96`, corpus v1 revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Split: `all`; counts by split: map[holdout:31]; counts by stratum: map[real-posse-review:31]
- Scored with corpus revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Started 2026-11-23T02:53:12Z, finished 2026-11-23T03:24:38Z, parallel 3

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 5% (1/21) |
| Recall | 7% (2/27) |
| Precision | 12% (2/17) |
| Allowed judgment findings (excluded from precision) | 0 |
| Priority accuracy | 50% (1/2) |
| False positives (clean / noise / all) | 4 / 0 / 15 |
| False positives judged a different defect at an expected location | 5 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 1 |
| Inconclusive reviews | 1/31 |
| Cost (Pi catalog pricing) | $2.0079 |
| Latency total / mean / max | 4925.2s / 149.2s / 173.4s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| posse-pr-1131-r1 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0852 | 170.4s |
| posse-pr-1131-r2 | outstanding-findings | 0/1 | 1 | 0 | 0/0 | $0.1000 | 173.4s |
| posse-pr-1146-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0176 | 132.1s |
| posse-pr-1146-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0225 | 149.9s |
| posse-pr-1152-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0673 | 159.8s |
| posse-pr-1152-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0669 | 157.4s |
| posse-pr-1156-r1 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0825 | 161.6s |
| posse-pr-1156-r2 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0822 | 161.7s |
| posse-pr-1158-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0902 | 156.6s |
| posse-pr-1158-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0850 | 161.0s |
| posse-pr-1159-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0515 | 165.7s |
| posse-pr-1159-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0511 | 161.0s |
| posse-pr-1164-r1 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0546 | 161.9s |
| posse-pr-1164-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0807 | 161.3s |
| posse-pr-1165-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0320 | 155.7s |
| posse-pr-1165-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0190 | 158.5s |
| posse-pr-1167-r1 | eval-error | 0/0 | 0 | 1 | 0/0 | $0.0000 | 0.0s |
| posse-pr-1167-r2 | eval-error | 0/0 | 0 | 1 | 0/0 | $0.0000 | 0.0s |
| posse-pr-1168-r1 | outstanding-findings | 0/2 | 2 | 2 | 0/0 | $0.0678 | 162.4s |
| posse-pr-1168-r2 | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0629 | 162.7s |
| posse-t1222-30c14997 | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0689 | 161.7s |
| posse-t1222-41abce87 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0677 | 153.0s |
| posse-t1222-4210ed17 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0305 | 148.3s |
| posse-t1222-9de124d7 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0537 | 159.3s |
| posse-t1222-a5e704a0 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.1024 | 155.4s |
| posse-t1222-bd2af87f | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0711 | 157.3s |
| posse-t1222-c6403c40 | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0730 | 169.9s |
| posse-t1222-c8af1296 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0432 | 156.5s |
| posse-t1222-e7d4350a | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0420 | 138.5s |
| posse-t1222-e86dccac | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0750 | 162.5s |
| posse-t1222-eb193df8 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.0747 | 156.1s |
| posse-task-1241-r1 | outstanding-findings | 1/3 | 0 | 0 | 0/1 | $0.0972 | 163.6s |
| posse-task-1241-r2 | outstanding-findings | 0/3 | 1 | 1 | 0/0 | $0.0893 | 169.9s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
