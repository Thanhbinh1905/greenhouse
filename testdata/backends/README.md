# Recorded backend outputs

Each case is the exact stdout, stderr and exit code of one real CLI run, recorded on 2026-11-08 at 20:47 local time (+07:00) with Claude Code 2.1.319, codex-cli 0.166.0 and Pi 1.5.0. `backend_classify_test.go` classifies every case; `fake-cli.py` replays them as fake `claude` and `codex` executables for the executable tests and `tools/terminal_check.py`.

How each case was produced:

- `*/success`: a real call through the User's login. Claude used `--json-schema`, Codex `--output-schema` with `RUST_LOG=codex_exec=info`.
- `claude/auth`: `claude -p` with `HOME` set to an empty directory.
- `codex/auth`: `codex exec` with `CODEX_HOME` set to an empty directory; the endpoint answered 401.
- `pi/auth`: `pi` with an empty key variable.
- `quota`, `rate-limit`, `insufficient-quota`, `unavailable` and `api-rate-limit`: the real CLI pointed at a local HTTP server that answered every request with a 429 or 503 response in the provider's error format. A real quota could not be provoked on demand. `claude/quota` used `CLAUDE_CODE_OAUTH_TOKEN` with a dummy value, so Claude Code applied its subscription limit handling and emitted `rate_limit_event`; `claude/api-rate-limit` used `ANTHROPIC_API_KEY` with a dummy value. `codex/quota-date` set the reset three days ahead.
- `connection-refused` and `waiting-for-network`: the CLI pointed at a closed local port. Codex retried until `timeout 120` stopped it (exit 124).

Paths are rewritten to `/scratch` and `/home/user`, key-like strings are replaced, and Claude's `credential_expires_at` value is replaced.
