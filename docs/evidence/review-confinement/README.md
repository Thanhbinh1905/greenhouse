# Review snapshot, resolution responses and confinement captures

Real PTY captures at 56, 80 and 100 columns from `python3 tools/terminal_check.py <binary> <dir> responses`, with an isolated state home, real Git and fake Pi agents. Each run applies `auto full 6`, prepares, runs the pipeline and inspects the result. The reviewer reports a finding on `sample.txt`, the fixer rewrites it and answers `fixed`, and the next review confirms the response.

- `before-*.log`: the binary built from `main` (`e41d0d3`). The reviewer runs in an empty directory, so the fake reviewer cannot open `sample.txt` and the review stops as `inconclusive`. The screen has no snapshot, confinement or response lines.
- `after-*.log`: this branch. The run ends `locally-ready` with `REPAIRED PASS`. The finding reads `Resolved (fixed)` with `Response fixed` and `Review check confirmed`. The review shows `Snapshot: unchanged | 1 file | tree ...` and the digest, and both agents show `Confinement: ... landlock enforced (ABI 8)` with their writable paths and allowed port.
- `red-resolution.log`: the resolution tests run against the previous commit (`026fff1`), before responses existed. Each fails because the repair carries no responses or a dispute is committed.

The full `tools/terminal_check.py` run, which also covers every earlier flow at 56, 80 and 100 columns, passed against this branch.
