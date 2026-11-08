# Configuration versions, repository trust and transcript captures

Real PTY captures at 56, 80 and 100 columns, recorded with isolated state homes and real Git.

- `before-*.log` and `after-*.log`: the same keystrokes (`u migrate`, `t trust`, `n new`, `s`, `o`) against the binary built from `main` before this change and from this branch. The repository holds `.greenhouse.json` and an unversioned `reviewer.json`. Before, the screen reports overrides as unsupported and the keys only refresh. After, it reports the version-1 file, migrates it, shows the override as ignored until trusted and then applied, lists each value's source, and opens the empty transcript log.
- `pty-*-trust.log` and `pty-*-logs.log`: the `tools/terminal_check.py` flow that migrates, trusts, saves to the repository override, applies, reviews, and shows the redacted transcript.
