# Review evaluation: 03 numbered candidate files and no-checkout contract

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `f64d4cf0332edab56670fecc6a8656de9d6045ef`, corpus v1 revision 0 `77d755d3549d8dacd11d1555593a2dc02b18acaf4c580f5251a96867f06298b3`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T06:13:50Z, finished 2026-11-08T06:14:36Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 100% (13/13) |
| Precision | 87% (13/15) |
| Priority accuracy | 100% (13/13) |
| False positives (clean / noise / all) | 1 / 1 / 2 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 1 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0137 |
| Latency total / mean / max | 141.4s / 8.8s / 15.1s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 7.1s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 6.4s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 7.4s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0009 | 12.4s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 7.0s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0010 | 10.9s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 6.2s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 6.3s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 6.5s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0007 | 4.8s |
| clean-directive-and-docs | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0013 | 15.1s |
| clean-extract-helper | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0011 | 12.3s |
| noise-legacy-context | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0009 | 8.6s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0005 | 4.4s |
| policy-needless-comments | outstanding-findings | 3/3 | 0 | 0 | 3/3 | $0.0011 | 11.8s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0009 | 14.2s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
