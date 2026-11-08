# Review evaluation: 07 model comparison: openai/gpt-6.1-sol run 2, product at 1edd8ba

- Model: `openai/gpt-6.1-sol` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `1edd8ba677f7ba0ea760afc87fbc90e1dbc9efee`, corpus v1 revision 2 `4f1ccef6d618a0feed8ff79f0276c70beb1e353268979213149c4095a3c50812`
- Scored with corpus revision 3 `88ba03b56bc40a0e621291fca524ea4cbc76ab732f85b580067c6cbf59001619`
- Started 2026-11-08T06:30:56Z, finished 2026-11-08T06:31:37Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 85% (11/13) |
| Precision | 79% (11/14) |
| Priority accuracy | 100% (11/11) |
| False positives (clean / noise / all) | 1 / 1 / 3 |
| False positives at an expected location naming another defect | 1 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.1153 |
| Latency total / mean / max | 138.7s / 8.7s / 13.5s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0073 | 8.6s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0072 | 8.5s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0069 | 7.6s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0080 | 10.9s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0070 | 7.6s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0075 | 8.9s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0075 | 7.7s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 1 | 0 | 1/1 | $0.0097 | 13.5s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0066 | 9.4s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0058 | 4.3s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0064 | 6.4s |
| clean-extract-helper | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0076 | 8.7s |
| noise-legacy-context | outstanding-findings | 0/0 | 1 | 0 | 0/0 | $0.0081 | 10.3s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0046 | 5.6s |
| policy-needless-comments | outstanding-findings | 1/3 | 0 | 0 | 1/1 | $0.0081 | 12.6s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0070 | 8.0s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
