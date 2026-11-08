# Review evaluation: 02 event parsing fix

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `1c944db9e9171f6faa8c89f99599844a468e034d`, corpus v1 revision 0 `77d755d3549d8dacd11d1555593a2dc02b18acaf4c580f5251a96867f06298b3`
- Scored with corpus revision 3 `88ba03b56bc40a0e621291fca524ea4cbc76ab732f85b580067c6cbf59001619`
- Started 2026-11-08T06:09:55Z, finished 2026-11-08T06:11:19Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **not met**

| Metric | Value |
|---|---|
| High-severity recall | 89% (8/9) |
| Recall | 92% (12/13) |
| Precision | 86% (12/14) |
| Priority accuracy | 100% (12/12) |
| False positives (clean / noise / all) | 1 / 0 / 2 |
| False positives at an expected location naming another defect | 1 |
| Blocking results on clean cases | 1 |
| Inconclusive reviews | 2/16 |
| Cost (Pi catalog pricing) | $0.0228 |
| Latency total / mean / max | 293.0s / 18.3s / 28.1s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | inconclusive | 0/1 | 0 | 0 | 0/0 | $0.0013 | 19.7s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0014 | 18.6s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0017 | 22.2s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 1 | 0 | 1/1 | $0.0019 | 24.5s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0011 | 11.9s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0016 | 22.6s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0015 | 18.7s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0020 | 28.1s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0014 | 21.3s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0006 | 4.7s |
| clean-directive-and-docs | inconclusive | 0/0 | 0 | 1 | 0/0 | $0.0013 | 16.3s |
| clean-extract-helper | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0012 | 16.3s |
| noise-legacy-context | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0011 | 14.3s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0013 | 12.0s |
| policy-needless-comments | outstanding-findings | 3/3 | 0 | 0 | 3/3 | $0.0017 | 21.5s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0019 | 20.3s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
