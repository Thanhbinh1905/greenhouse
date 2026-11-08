# Review evaluation: 09 model comparison: openai/gpt-6-luna run 2, product at 1edd8ba

- Model: `openai/gpt-6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `1edd8ba677f7ba0ea760afc87fbc90e1dbc9efee`, corpus v1 revision 2 `4f1ccef6d618a0feed8ff79f0276c70beb1e353268979213149c4095a3c50812`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T06:31:58Z, finished 2026-11-08T06:32:18Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 89% (8/9) |
| Recall | 85% (11/13) |
| Precision | 85% (11/13) |
| Priority accuracy | 100% (11/11) |
| False positives (clean / noise / all) | 0 / 1 / 2 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0045 |
| Latency total / mean / max | 67.8s / 4.2s / 5.9s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.9s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.0s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 5.0s |
| bug-pagination-bounds | outstanding-findings | 0/1 | 1 | 1 | 0/0 | $0.0003 | 4.8s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.5s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.7s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.5s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.9s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.2s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0003 | 2.5s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0003 | 2.7s |
| clean-extract-helper | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0002 | 3.7s |
| noise-legacy-context | outstanding-findings | 0/0 | 1 | 1 | 0/0 | $0.0003 | 4.3s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0002 | 2.6s |
| policy-needless-comments | outstanding-findings | 2/3 | 0 | 0 | 2/2 | $0.0003 | 5.9s |
| policy-unslop-readme | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0003 | 4.7s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
