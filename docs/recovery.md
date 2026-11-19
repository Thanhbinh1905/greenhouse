# Recovery after interruption

When the daemon dies during work, restart adopts the results that durable records prove. Every other interrupted step waits for your answer and is never retried.

## Executor records

Every reviewer, fixer, check, stage command, and the pipeline's sync and push Git commands, run through the runner. Each one gets an executor record under `GREENHOUSE_HOME/executors/`. Controller Git writes (candidate creation, the conflict-fix commit, handback and recovery refs) do not go through the runner and are reconciled from refs.

| File | Contents |
| --- | --- |
| `<id>.lease` | Held with `flock`. The runner passes the descriptor to the child. |
| `<id>.out`, `<id>.err` | Bounded output. |
| `<id>.json` | The exit code, input digest, stop reason and output byte counts, and `reaped`: whether the runner verified that it reaped the whole process tree. Written after the reap attempt; a record with `reaped: false` shows as `unverified`. |

When the daemon dies, its executors keep running until they finish or reach the configured [`executor_timeout`](axi.md#executor-timeout) deadline (default `45m`), plus the 30-second supervisor cleanup allowance. Nothing signals them by PID.

## What restart adopts

On restart, and every 250 ms while an orphaned executor still holds its lease, the daemon probes the open recovery:

- An interrupted review whose record is complete and whose input digest matches is adopted as if it had finished live. Its budget stays consumed.
- An interrupted command stage is adopted with the recorded exit codes and output when all its commands have complete records and the candidate is clean at the recorded head. A full pipeline then parks (`parked`) until `pipeline` continues it.
- Everything else stays `recovery-required`. This includes every fixer, conflict fix, target merge, candidate creation, push and handback.

## Recovery evidence

`run.recovery` holds the evidence:

- the kind and stage
- each executor's state: `running`, `exited`, `stopped`, or `unverified` when the runner left no complete record
- for a candidate: its ref, the recorded and observed heads, cleanliness and merge state

If the runner itself is killed, a descendant that closed the inherited descriptor can outlive the lease undetected. `unverified` marks that risk.

The terminal shows the same evidence under `RECOVERY REQUIRED`. Answer it with `e` in the line client.

## Answer a recovery

AXI uses `recover` with `gate_id` set to `run.recovery.id`, a `choice` and a `reason`:

```json
{"version":1,"op":"recover","run_id":"...","input_id":"...","gate_id":"recovery id","expected":"token","action_id":"unique","choice":"adopt","reason":"the commit matches the checked repair"}
```

| Choice | Effect |
| --- | --- |
| `inspect` | Probes again and records a `recovery-inspect` event. |
| `adopt` | Accepts the observed result. A full pipeline continues from there. |
| `discard` | Keeps an advanced head at `refs/greenhouse/recovered/<recovery id>` and uncommitted changes in `run.recoveries[].partial`. Resets the private candidate to the recorded head and reruns the stage. The rerun spends new budget; when the budget refuses it, the run stays stopped at that stage with the reason. Interrupted handbacks cannot be discarded. |
| `new-run` | Marks the run `superseded`, keeps its records and candidate ref, and links a successor. The successor inherits the consumed review and repair usage, the gate repair usage and any final-fix grant; `run.inherited` records them. The successor can be prepared again until it dispatches its own review, and Prepare keeps the inherited usage and grant. |

`adopt` is offered only when no executor still holds a lease and the observed candidate matches what the durable record predicted:

- a repair commit whose parent is the repair base and whose diff equals the checked diff, with every configured check passed
- a conflict or target merge whose parents are the recorded candidate and target, and, for a conflict, without markers
- an intact new candidate worktree
- after an interrupted handback, a source branch that is unchanged or already at the candidate

Each answer is stored in `run.recoveries` with its choice, actor, reason, action ID and outcome. It changes the token and the gate, so a stale or repeated answer is refused or replays its receipt. No choice refunds budget.

## Candidate custody

Candidate custody and the writer lock are scoped to the source branch, or to the worktree when its head is detached.

Runs on different branches of one repository, each in its own linked worktree, can review, repair, test, push and open PRs at the same time. Each run writes only its own `refs/heads/greenhouse/<id>` candidate ref and fast-forwards only its own source branch.

A second run on the same branch is refused while the first holds custody. An example is a worktree added with `git worktree add --force`.

Writers take a non-blocking `flock` on a file under `greenhouse-writers/` in the repository's common Git directory, one file per branch. A writer from another state home on the same branch therefore fails with `writer-conflict`.

## Candidate leases

A lease file in the repository's common Git directory, under `greenhouse-leases/`, guards each candidate ref. The lease names the owning state home and an epoch, and survives daemon restarts.

- Creating a candidate takes the lease.
- Every fixer start, commit, target merge, conflict fix and recovery write first checks that this state home holds the lease at the epoch the run recorded.
- A daemon on another state home, or a restored copy of an older store, is refused.
- `discard` moves the lease to the next epoch.
- The daemon removes a lease of its own state home only when the run holding that exact epoch has released custody and has no running executor.

Runs created before leases stay unleased.
