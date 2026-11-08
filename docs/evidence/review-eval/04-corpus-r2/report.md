# Review evaluation: 04 corpus revision 2, product at bd9dd6a

- Model: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0
- Greenhouse: `bd9dd6abca7dcd3efe5517598677f7d135c5dca2`, corpus v1 revision 2 `4f1ccef6d618a0feed8ff79f0276c70beb1e353268979213149c4095a3c50812`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T06:15:25Z, finished 2026-11-08T06:16:01Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 92% (12/13) |
| Precision | 92% (12/13) |
| Priority accuracy | 100% (12/12) |
| False positives (clean / noise / all) | 0 / 0 / 1 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Pi catalog pricing) | $0.0131 |
| Latency total / mean / max | 122.7s / 7.7s / 15.1s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0009 | 8.0s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 6.0s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 7.8s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0010 | 10.8s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 6.3s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0009 | 9.2s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0009 | 10.4s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0008 | 7.5s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0007 | 5.5s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0007 | 4.2s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0008 | 5.4s |
| clean-extract-helper | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0009 | 9.3s |
| noise-legacy-context | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0006 | 4.6s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0005 | 3.0s |
| policy-needless-comments | outstanding-findings | 2/3 | 0 | 0 | 2/2 | $0.0013 | 15.1s |
| policy-unslop-readme | outstanding-findings | 1/1 | 1 | 0 | 1/1 | $0.0009 | 9.6s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to Pi on stdin, `argv.json` the exact arguments, and `events.jsonl` Pi's raw JSON event stream.
