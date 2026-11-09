# Fixer evaluation: baseline: fixer instructions as on main

- Fixer: `openai/gpt-5.6-luna` through `pi-json-v1`, Pi 1.5.0; the review is seeded from each case
- Greenhouse: `d9b2d9a9c1d5cb5cfc5e43f122fe7cb0d90a85f0`, fixer corpus v1 revision 1 `18abdc4ea64b9b609bc405f0e0a458e8f25f0970c95b16a693c0e25a1ee9aaa2`
- Started 2026-11-09T02:28:16Z, finished 2026-11-09T02:29:02Z, parallel 4

Bar (at least 75% of cases fixed, no regression, no scope violation): **not met**

| Metric | Value |
|---|---|
| Fixed | 80% (8/10) |
| Hidden acceptance tests passing | 10/10 |
| Regressions (cases / failing tests) | 0 / 0 |
| Scope violations (cases / edits) | 2 / 2 |
| Correct resolution responses | 10/10 |
| Cost | $0.0115 |
| Fixer latency total / mean / max | 120.8s / 12.1s / 14.9s |

| Case | Repair status | Response | Acceptance | Regressions | Scope violations | Fixed | Cost | Latency |
|---|---|---|---|---|---|---|---|---|
| dispute-guarded-division | responded-no-change | disputed | 1/1 | none | none | true | $0.0009 | 9.3s |
| fix-case-insensitive | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0009 | 8.6s |
| fix-dropped-decode-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0010 | 10.7s |
| fix-empty-average | fixed-not-independently-rereviewed | fixed | 1/1 | none | changed stats/stats.go:26 | false | $0.0012 | 14.8s |
| fix-needless-comments | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0012 | 13.2s |
| fix-nil-map-write | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 13.9s |
| fix-offbyone-window | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0010 | 10.4s |
| fix-pagination-bounds | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0016 | 13.4s |
| fix-swallowed-write-error | fixed-not-independently-rereviewed | fixed | 1/1 | none | none | true | $0.0011 | 11.7s |
| fix-unlock-missing | fixed-not-independently-rereviewed | fixed | 1/1 | none | changed cache/cache.go:24 | false | $0.0014 | 14.9s |

Per-case `input.json` holds the exact bytes Greenhouse delivered to the fixer on stdin, `argv.json` the exact arguments, and `events.jsonl` the fixer's JSON event stream. `report.json` holds each repair diff, check result, resolution response and hidden test outcome.
