# Review evaluation: 08 model comparison: openai/gpt-6-luna run 1, product at 1edd8ba

- Model: `openai/gpt-6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `1edd8ba677f7ba0ea760afc87fbc90e1dbc9efee`, corpus v1 revision 2 `4f1ccef6d618a0feed8ff79f0276c70beb1e353268979213149c4095a3c50812`
- Started 2026-11-08T06:31:37Z, finished 2026-11-08T06:31:58Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 92% (12/13) |
| Precision | 92% (12/13) |
| Priority accuracy | 100% (12/12) |
| False positives (clean / noise / all) | 0 / 1 / 1 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0046 |
| Latency total / mean / max | 73.8s / 4.6s / 7.1s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.8s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.6s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 7.1s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 5.1s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.6s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 5.0s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.7s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 5.3s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.9s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0003 | 3.0s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0003 | 2.9s |
| clean-extract-helper | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0002 | 2.8s |
| noise-legacy-context | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0003 | 4.8s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0002 | 2.8s |
| policy-needless-comments | outstanding-findings | 2/3 | 0 | 0 | 2/2 | $0.0003 | 6.3s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 5.2s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
