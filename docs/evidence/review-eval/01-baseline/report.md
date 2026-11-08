# Review evaluation: 01 baseline: unchanged product reviewer

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `453d1380267cd2325c8199be642618088cc5a981`, corpus v1 revision 0 `77d755d3549d8dacd11d1555593a2dc02b18acaf4c580f5251a96867f06298b3`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T06:08:17Z, finished 2026-11-08T06:09:49Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 0% (0/9) |
| Recall | 0% (0/13) |
| Precision | n/a (0/0) |
| Priority accuracy | n/a (0/0) |
| False positives (clean / noise / all) | 0 / 0 / 0 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 3 |
| Inconclusive reviews | 16/16 |
| Cost (Pi catalog pricing) | $0.0251 |
| Latency total / mean / max | 318.5s / 19.9s / 38.3s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0013 | 16.5s |
| bug-nil-before-error | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0013 | 17.5s |
| bug-offbyone-window | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0014 | 19.9s |
| bug-pagination-bounds | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0014 | 18.6s |
| bug-race-word-count | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0014 | 18.7s |
| bug-shell-injection | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0026 | 38.3s |
| bug-sql-injection | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0014 | 18.2s |
| bug-swallowed-write-error | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0022 | 30.8s |
| bug-unlock-missing | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0016 | 24.3s |
| clean-bounds-fix-with-test | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0006 | 4.7s |
| clean-directive-and-docs | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0015 | 13.1s |
| clean-extract-helper | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0019 | 25.9s |
| noise-legacy-context | inconclusive | 0/0 | 0 | 0 | 0/0 | $0.0017 | 14.4s |
| noise-typo-beside-injection | inconclusive | 0/0 | 0 | 0 | 0/0 | $0.0015 | 14.0s |
| policy-needless-comments | inconclusive | 0/3 | 0 | 0 | 0/0 | $0.0023 | 30.2s |
| policy-unslop-readme | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0011 | 13.3s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
