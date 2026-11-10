# Follower-authoritative stream ordering (t94)

A delayed response from an unseen intermediate daemon must not replace the adopted stream. Only the follower introduces a stream. Action responses must match that stream and satisfy sequence ordering; retired streams remain rejected.

Both terminal clients use `snapshotOrder.accepts`. The line client checks ordinary responses and its intermediate agent-selection Apply response. A cross-stream prerequisite stops the next Run or intent step until follower resync. Receipts still produce feedback, and resync does not replay the action.

Follower adoption no longer depends on a digest that omits stream and sequence. A daemon restart can preserve every visible field while changing the stream; the client must still adopt its stream and cursor.

## Regression evidence

`TestDelayedIntermediateDaemonResponse` reproduces t93 through the TUI's keyboard, Update, and response handlers: display A, initiate cancellation handled by B, adopt C, then deliver B. It also covers B's response before C's watch snapshot. Both preserve follower authority.

The regression fails against production sources from `75b380c5c945adf1c7631c352416a9fdc57cacbb`:

```text
delayed intermediate daemon response replaced newer state: stream=daemon-B token=token-B status=cancelling
```

`TestLineDelayedIntermediateDaemonResponse` drives the real line client in a subprocess using stdin, a controlled Unix-socket fixture, and rendered stdout. Both arrival orders fail on the base because the screen shows `cancelling` from B. Both pass on the fix and reach `cancelled` from C. The blocking line-client request leaves the follower update queued until the response returns; the regression covers that path.

The six existing ordering and reconnect tests remain. The old assertion that a response introduces a stream now requires a follower snapshot before checking retired-stream rejection. Additional tests cover an unchanged screen across restart, a chained Run prerequisite, same-stream sequences, and retired streams.

No protocol state, action authority, trust, budget, review, or delivery changed. No dependency changed. Rollback: revert this Task's commit.

## Validation

The Task report records exact commands, results, timings, captures, and cleanup. Logs and captures are under `/home/thanhbinh/.posse/scratch/greenhouse/t94`. `red.log` contains the base-overlay failure; `green.log` contains the focused passing checks. All heavyweight checks use CPUs 10-11, niceness 15, `GOMAXPROCS=2`, and `GOFLAGS='-p=1'`. Go tests also use `-p 1 -parallel 1` and a finite timeout.

All required checks passed: `gofmt`, `git diff --check`, `go vet ./...`, `staticcheck ./...`, full tests, race tests, and all five terminal harnesses. Full tests took 427.61 seconds; race validation took 535.96 seconds. TUI captures show cancellation and reattachment, daemon loss with actions disabled, authoritative resync, and the existing editing and fallback flows.

The event harness left three fixture daemons, matching the cleanup issue already documented in the t87 evidence. Each process was verified by executable and `GREENHOUSE_HOME`, then stopped before the TUI check. The final process scan found no remaining Task-owned fixtures.
