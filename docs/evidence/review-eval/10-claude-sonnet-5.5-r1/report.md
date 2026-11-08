# Review evaluation: Claude backend, claude-sonnet-5-5

- Model: `claude-sonnet-5-5` through `claude-stream-json-v1`, Claude Code 2.1.319
- Greenhouse: `f3d54eb685addd07327518627b986d24cdfcbc49`, corpus v1 revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Scored with corpus revision 4 `9d7a6b7eeee9a092ccedc52a37724bed453e2e400c640cfd9f42fb293440c80b`
- Started 2026-11-08T14:15:04Z, finished 2026-11-08T14:15:33Z, parallel 4

Bar (high-severity recall >= 80%, no blocking result on clean cases): **met**

| Metric | Value |
|---|---|
| High-severity recall | 100% (9/9) |
| Recall | 100% (13/13) |
| Precision | 87% (13/15) |
| Priority accuracy | 100% (13/13) |
| False positives (clean / noise / all) | 0 / 0 / 2 |
| False positives judged a different defect at an expected location | 0 |
| Eligible findings without an adjudication record (not credited) | 0 |
| Blocking results on clean cases | 0 |
| Inconclusive reviews | 0/16 |
| Cost (Claude Code list pricing) | $0.2710 |
| Latency total / mean / max | 92.9s / 5.8s / 7.1s |

| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |
|---|---|---|---|---|---|---|---|
| bug-empty-average | outstanding-findings | 1/1 | 1 | 0 | 1/1 | $0.0153 | 6.9s |
| bug-nil-before-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0253 | 6.1s |
| bug-offbyone-window | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0253 | 6.6s |
| bug-pagination-bounds | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0142 | 6.1s |
| bug-race-word-count | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0256 | 6.2s |
| bug-shell-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0144 | 6.0s |
| bug-sql-injection | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0148 | 6.1s |
| bug-swallowed-write-error | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0256 | 5.4s |
| bug-unlock-missing | outstanding-findings | 1/1 | 0 | 0 | 1/1 | $0.0132 | 5.7s |
| clean-bounds-fix-with-test | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0144 | 5.1s |
| clean-directive-and-docs | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0170 | 6.2s |
| clean-extract-helper | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0126 | 4.0s |
| noise-legacy-context | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0119 | 4.6s |
| noise-typo-beside-injection | clean-review | 0/0 | 0 | 0 | 0/0 | $0.0112 | 4.8s |
| policy-needless-comments | outstanding-findings | 3/3 | 0 | 0 | 3/3 | $0.0153 | 6.1s |
| policy-unslop-readme | outstanding-findings | 1/1 | 1 | 0 | 1/1 | $0.0149 | 7.1s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `claude-stream-json-v1`.
