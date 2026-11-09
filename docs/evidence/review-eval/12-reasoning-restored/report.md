# Review evaluation: Preserve catalog reasoning in private provider entry

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `f2814950295a4f1a917303916c87cc70efcbd419`, corpus v1 revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-09T07:07:36Z, finished 2026-11-09T07:09:04Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 100% (13/13) |
| Precision | 93% (13/14) |
| Priority accuracy | 100% (13/13) |
| False positives (clean / noise / all) | 1 / 0 / 1 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0000 |
| Latency total / mean / max | 230.7s / 14.4s / 26.6s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 13.5s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 13.5s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 18.0s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 11.3s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 17.1s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 18.1s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 8.7s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 12.3s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 14.1s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 6.5s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 11.5s |
| clean-extract-helper | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0000 | 17.3s |
| noise-legacy-context | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 19.6s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 5.9s |
| policy-needless-comments | outstanding-findings | 3/3 | 0 | 0 | 3/3 | $0.0000 | 26.6s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 16.7s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
