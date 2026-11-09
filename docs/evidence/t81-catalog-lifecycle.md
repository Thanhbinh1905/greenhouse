# P-1: catalog discovery follows the execution lifetime

Pi model discovery during provider preparation now runs through Greenhouse's process-tree runner. The lookup inherits the review or repair cancellation context, has a four-second deadline, and records its output and exit under `<execution>.catalog.<sequence>`. The runner stops and reaps descendants before returning. Provider-file writing takes the discovered reasoning flag and starts no process.

Cancel is available during a repair's candidate and provider preparation as well as fixing and checking. Reviewer preparation and conflict-repair preparation propagate cancellation too. A discovery timeout prevents the agent from starting. An unknown process-tree outcome stays an unknown outcome for repair recovery. Review adoption retains catalog evidence and requires completed catalog records before using the separate verdict record.

## Executable evidence

`TestBlockedCatalogPreparationCanBeCancelled` drives the real executable over AXI with real Git. Its fixture lets the review's admission preflight finish, then blocks the runtime catalog lookup and spawns a child. It covers both reviewer and fixer roles, with explicit Cancel and the automatic deadline. The tests check the final status and the catalog executor's stopped and reaped record.

Against `63eef28`, the cancellation cases failed:

- Reviewer: `reviewer catalog cancellation did not settle: cancelling`.
- Fixer: `cancel during fixer provider preparation refused: unsafe replacement or mutation; explicit recovery required`.

The four catalog processes left by that red-before reproduction received SIGTERM and exited. The restored implementation passed the cancellation and timeout cases, the reasoning-record tests, and the executable restart snapshot test together in 26.627s. No test-owned processes remained after that green run.

Verification uses CPUs 10 and 11, niceness 15, `GOMAXPROCS=2`, and `GOFLAGS=-p=1`. Go tests also pass `-p 1 -parallel 1`. Every verification command runs sequentially. Runtime discovery changes process supervision; the reasoning flag and agent inputs are unchanged, so the recorded paid evaluation results are reused.

## Full checks

Under the same resource envelope, `go test -p 1 -parallel 1 -timeout 30m ./...` passed in 411.970s for Greenhouse and 21.567s for eval. `go test -race -p 1 -parallel 1 -timeout 30m ./...` passed in 478.245s and 28.181s. Neither command timed out. Cleanup scans after each found no test-owned processes. A live scan during the race suite confirmed its test binary and nested race build remained on CPUs 10 and 11.

`go vet ./...` passes. Staticcheck first flagged `string(out.Bytes())` as S1030; replacing it with the equivalent `out.String()` resolved the warning. `staticcheck ./...` now passes.


The terminal, recovery, and events PTY checks passed at 56, 80, and 100 columns, sequentially under the same resource envelope. Terminal and recovery checks left no fixture processes. The events check repeated its known cleanup leak: three verified fixture daemons remained after success. They received SIGTERM and exited; no unexpected task processes remained. The first cleanup scan stopped at a `/proc/1/cwd` permission error; the corrected read-only scan skipped inaccessible system processes and completed cleanup. No further heavyweight verification started after the leak.

Task scratch logs are `catalog-red-runtime.log`, `catalog-targeted.log`, `p1-suite.log`, `p1-race.log`, `p1-vet.log`, `p1-staticcheck.log`, `p1-terminal.log`, `p1-recovery.log`, and `p1-events.log`. PTY captures are in `p1-pty/`. Formatting and `git diff --check` pass. The final diff was checked for scope, slop, and needless comments.
