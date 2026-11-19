# State and daemon

One daemon per state directory keeps your runs. Clients attach to it, and the daemon keeps working after they detach.

## State directory

`GREENHOUSE_HOME` overrides the default `~/.local/state/greenhouse`. The state directory must be private, with mode 0700.

The Unix socket path must fit the platform's Unix-socket path limit. Use a short state path in deeply nested environments.

## Store and durability

One daemon holds an advisory `flock` before it reads recovery state or binds the socket. It never unlinks a live listener.

Mutations execute serially. Each mutation commits the entire version-1 store before responding:

1. Write and sync a temporary file in the same directory.
2. Rename it over the store.
3. Sync the directory.

Action receipts and run changes share that commit. Archived runs and receipts are written as separate files under `history/` before the store.

Read requests do not commit. During a commit, `inspect` and `transcript` may answer from the last committed state.

Startup parks persisted `running`, `cancelling` or `writing` runs as `recovery-required` and reconciles them as described in [recovery](recovery.md). It never reruns an agent or a check. The exception is an interrupted push, PR or CI step: startup resets it to pending and resumes it without an agent, because the step checks the remote state first. Invalid run records and unknown store versions refuse startup.

Rule snapshots are an additive version-1 extension: old stores need no rewrite or counter migration. New snapshots validate source, exact bytes, digest, parsed criteria and configuration identity on load.

## Assumptions and limits

This design assumes a local file system with reliable `flock`, atomic rename and fsync. It does not support:

- network file systems
- concurrent external store editors
- cross-user access
- online migration

Protocol and configuration versions follow the [compatibility policy](compatibility.md).

- Disk corruption has no automatic repair.
- A directory-sync failure can leave an uncertain commit. Inspect after restart instead of inventing another mutation.
- Back up the state directory while the daemon is stopped.
- The store keeps the active run of each worktree. On each save, finished runs and all action receipts move to `history/runs/` and `history/receipts/`. Each kind keeps at most 128 files or 64 MiB; the oldest files beyond that are deleted. A request for a pruned run fails with `historical data may have been pruned`.
- Storage also grows with events, snapshots and executor records, which are not compacted.

## Execution and cancellation

The daemon commits review or repair admission and budget consumption before it starts execution. Completion returns through the serialized controller. Inspection stays available during execution.

- Review cancellation stops its current child.
- Repair and check cancellation asks the Linux subreaper to terminate and reap descendants.
- A killed runner or unverified cleanup leaves an unknown outcome.
- Restart never signals stored PIDs, and never retries an agent or a check.

## What Greenhouse changes in Git

Review-stage Git commands are observational. The separately authorized one-pass repair creates a managed candidate branch, a worktree and a local commit. Guarded handback may fast-forward the unchanged source branch.

Greenhouse does not stash, reset, rebase, force-update, push or discard source work.

## Stop the daemon

Run `greenhouse stop` to stop the daemon for the current `GREENHOUSE_HOME`.

- It refuses while work is running. Let the work settle and retry.
- It reports when no daemon is running.
- For older daemons, it refuses and gives [manual shutdown instructions](compatibility.md#negotiation). It never signals the process.

SIGTERM handling stays available. A client exit does not shut down the daemon.

## Transcript logs

Normal use never needs a transcript: findings, verdicts and bounded summaries are in the run.

For diagnosis, each reviewer and fixer invocation appends its raw stdout and stderr to `transcripts/<run-id>.log` in the state directory, with mode 0600.

Before writing, Greenhouse redacts:

- values of 8 or more characters from environment variables whose names contain `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `PASSWD`, `AUTH`, `CREDENTIAL`, `COOKIE` or `SESSION`, replaced with `[REDACTED:<NAME>]`
- bearer tokens
- `key=value` and `token: value` style secrets
- common API-key shapes

A run's log stops at 4 MiB with an omission marker.

AXI `transcript` returns the whole log of the current run, or of `run_id`, and the last 16 KiB of each stage log in `steps`. In the terminal, `o` shows the last 40 lines of the transcript and of each stage log.

## Stage logs

Each stage's redacted output goes to `logs/<run-id>/<step>.log` in the state directory, with mode 0600 and at most 4 MiB per step. The steps are `review`, `repair`, `sync`, `test`, `docs`, `lint`, `push`, `pr` and `ci`. AXI `logs` reads a whole step log; see [read output through Greenhouse](worker-contract-v1.md#read-output-through-greenhouse).
