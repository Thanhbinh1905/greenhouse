# Live backend smoke review

Date: 2026-11-08. Greenhouse at the t40 branch, built from the working tree. Claude Code 2.1.319, codex-cli 0.166.0, Pi 1.5.0.

Each backend ran one one-pass review through the product path: an isolated `GREENHOUSE_HOME`, `axi` `attach`, `apply`, `resolve-rules`, `prepare` and `review`, then `inspect` until the run settled. The change under review adds a two-line Python `average` helper. Claude and Codex used the existing CLI logins on this machine. Codex reached its login's endpoint through `provider.base_url` in `backends.json`. Pi read its key from `OPENAI_API_KEY`, filled by `pi auth print-api-key` and never written to a file.

| Backend | Requested model | Execution backend | Reported provider / model | Review | Findings | Review rounds used | Cost | Tokens |
|---|---|---|---|---|---|---|---|---|
| `claude` | `claude-haiku-5-5` | `claude-stream-json-v1` | `claude` / `claude-haiku-5-5` | outstanding-findings | 1 | 1 | $0.0009 | 990 |
| `codex` | `gpt-5.6-luna` | `codex-json-v1` | `codex` / `gpt-5.6-luna` | clean-review | 0 | 1 | $0.0000 | 13682 |
| `pi` | `openai/gpt-5.6-luna` | `pi-json-v1` | `openai` / `gpt-5.6-luna` | clean-review | 0 | 1 | $0.0000 | 1775 |

All three reported exactly the requested model and covered `stats.py`. Claude flagged the empty-list division as a P3 note; Codex and Pi returned clean reviews. Codex reports tokens but no price. Pi reports no price for this custom provider.

The JSON files in [backend-smoke/](backend-smoke/) hold each run's summary as AXI returned it: statuses, reported model, findings, coverage, the attempt record, the bounded execution log, and the first 40 arguments of the exact argv. Raw transcripts are not stored.
