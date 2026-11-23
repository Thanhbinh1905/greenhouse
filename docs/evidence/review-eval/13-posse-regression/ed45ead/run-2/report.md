# Review evaluation: posse regression, build ed45ead, pass 2

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.2
- Greenhouse: `ed45ead399a81d95fa964b7ed3a35a25ef4b3c96`, corpus v1 revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Split: `all`; counts by split: map[holdout:31]; counts by stratum: map[real-posse-review:31]
- Scored with corpus revision 9 `7bc60e4f58a33f418a377b4d0e03b452fd6d19cc7f6958a792a0eb9e1f94a8a5`
- Started 2026-11-23T02:53:12Z, finished 2026-11-23T03:23:45Z, parallel 3

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 14% (3/21) |
| Recall | 15% (4/27) |
| Precision | 17% (4/23) |
| Allowed judgment findings (excluded from precision) | 0 |
| Priority accuracy | 75% (3/4) |
| False positives (clean / noise / all) | 8 / 0 / 19 |
| False positives judged a different defect at an expected location | 6 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 4 |
| Inconclusive reviews | 1/31 |
| Cost (Pi catalog pricing) | $2.1149 |
| Latency total / mean / max | 4934.9s / 149.5s / 174.7s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| posse-pr-1131-r1 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0965 | 164.7s |
| posse-pr-1131-r2 | outstanding-findings | 0/1 | 2 | 2 | 0/0 | $0.0988 | 167.9s |
| posse-pr-1146-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0147 | 153.7s |
| posse-pr-1146-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0168 | 153.7s |
| posse-pr-1152-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0264 | 158.6s |
| posse-pr-1152-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0690 | 159.4s |
| posse-pr-1156-r1 | clean-review | 0/1 | 0 | 0 | 0/0 | $0.1291 | 159.7s |
| posse-pr-1156-r2 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0869 | 159.8s |
| posse-pr-1158-r1 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0971 | 157.3s |
| posse-pr-1158-r2 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.1225 | 157.2s |
| posse-pr-1159-r1 | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0629 | 162.9s |
| posse-pr-1159-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0495 | 165.1s |
| posse-pr-1164-r1 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0408 | 150.2s |
| posse-pr-1164-r2 | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0699 | 157.8s |
| posse-pr-1165-r1 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0212 | 156.0s |
| posse-pr-1165-r2 | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0309 | 155.4s |
| posse-pr-1167-r1 | eval-error | 0/0 | 0 | 1 | 0/0 | $0.0000 | 0.0s |
| posse-pr-1167-r2 | eval-error | 0/0 | 0 | 1 | 0/0 | $0.0000 | 0.0s |
| posse-pr-1168-r1 | clean-review | 0/2 | 0 | 0 | 0/0 | $0.0752 | 152.0s |
| posse-pr-1168-r2 | outstanding-findings | 0/2 | 1 | 0 | 0/0 | $0.0673 | 156.9s |
| posse-t1222-30c14997 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0662 | 158.2s |
| posse-t1222-41abce87 | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0799 | 154.4s |
| posse-t1222-4210ed17 | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0428 | 156.9s |
| posse-t1222-9de124d7 | outstanding-findings | 1/1 | 0 | 0 | 0/1 | $0.0442 | 157.0s |
| posse-t1222-a5e704a0 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0806 | 153.7s |
| posse-t1222-bd2af87f | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0981 | 172.9s |
| posse-t1222-c6403c40 | outstanding-findings | 0/2 | 1 | 1 | 0/0 | $0.0696 | 174.7s |
| posse-t1222-c8af1296 | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0657 | 160.4s |
| posse-t1222-e7d4350a | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0513 | 155.8s |
| posse-t1222-e86dccac | clean-review | 0/2 | 0 | 0 | 0/0 | $0.1076 | 165.5s |
| posse-t1222-eb193df8 | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0850 | 156.8s |
| posse-task-1241-r1 | outstanding-findings | 0/3 | 1 | 1 | 0/0 | $0.0656 | 158.2s |
| posse-task-1241-r2 | clean-review | 0/3 | 0 | 0 | 0/0 | $0.0824 | 162.3s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
