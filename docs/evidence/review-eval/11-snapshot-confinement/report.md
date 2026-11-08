# Review evaluation: full-tree snapshot, resolution responses, Landlock confinement

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `91aa255c48176a8e9f753d0646c8c9af4e774acf`, corpus v1 revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T20:42:40Z, finished 2026-11-08T20:43:40Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 100% (13/13) |
| Precision | 81% (13/16) |
| Priority accuracy | 100% (13/13) |
| False positives (clean / noise / all) | 1 / 1 / 3 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0000 |
| Latency total / mean / max | 193.3s / 12.1s / 23.0s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 12.4s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 10.6s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 14.6s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 12.8s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 6.3s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 14.6s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 7.6s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 11.4s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 9.6s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 8.7s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 10.5s |
| clean-extract-helper | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0000 | 16.1s |
| noise-legacy-context | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0000 | 14.8s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0000 | 6.3s |
| policy-needless-comments | outstanding-findings | 3/3 | 1 | 0 | 3/3 | $0.0000 | 23.0s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0000 | 14.1s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `pi-json-v1`.
