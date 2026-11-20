# JSON interface

`greenhouse axi` reads one JSON request on stdin and writes one JSON envelope on stdout.

```sh
printf '%s\n' '{"version":2,"op":"attach"}' | /path/to/greenhouse axi
```

The invoking directory identifies the worktree. The CLI replaces any request `path` with that directory.

Related documents:

- [Worker contract v1](worker-contract-v1.md): AXI subcommands, progress, output, decisions and delivery verification for a full run without a terminal.
- [Embedding contract](embedding.md): complete requests, prepare fields and the grant-based review flow for a host.
- [Compatibility policy](compatibility.md): protocol versions.

## Versions

The daemon accepts protocol versions 1 and 2 and answers in the requested version. It reports its range in `protocol`. Version 2 adds `watch` and host mode.

## Worktree identity

Git identity commands discard inherited `GIT_*` variables. The daemon therefore cannot reuse the first client's repository selection for later clients. The invoking path determines identity, including for linked worktrees.

`attach` creates a run only when its canonical context has no active slot. `inspect` never creates a run. Reopening retained work does not consume budget, change snapshots or execute a stage.

New runs include `run.started_at` and `run.stage_started_at`, using RFC 3339 timestamps. `stage_started_at` changes whenever `run.stage` changes. Pipeline stages include `started_at` and `finished_at` once started. A running pipeline stage has `started_at` but no `finished_at`; a pending stage has neither. Older stored runs and stages omit timestamps that were not recorded.

## Operations

| Kind | Operations |
| --- | --- |
| Reads | `attach`, `inspect`, `watch`, `models`, `transcript`, `logs` |
| Mutations | `apply`, `save`, `trust`, `migrate`, `new`, `resolve-rules`, `prepare`, `review`, `repair`, `pipeline`, `resolve-conflict`, `decide`, `handback`, `cancel`, `resume`, `rerun`, `rerun-from-start`, `cancel-auto-resume`, `recover` |

`models` queries the trusted Pi executable's catalog. `transcript` returns a [transcript log](state.md#transcript-logs).

`logs` reads the redacted output of one stage, such as `test`. It takes `step`, and optionally `full`, `offset`, `follow` and `wait_ms`. A follow request waits for new output while the step is active. See [read output through Greenhouse](worker-contract-v1.md#read-output-through-greenhouse) for the steps, limits and response fields.

## Mutations

Copy `run_id`, `input_id`, `gate_id` and `token` from a response into the mutation. Send `token` as `expected`:

```json
{
  "version": 1,
  "op": "apply",
  "run_id": "from response",
  "input_id": "from response",
  "gate_id": "none",
  "expected": "from response token",
  "action_id": "a unique stable client action ID",
  "reason": "increase the run limit",
  "settings": {
    "mode": "ask",
    "preset": "one-pass",
    "limit": 6,
    "rules": ["unslop", "no-comments"],
    "reviewer_backend": "pi",
    "reviewer_model": "openai/gpt-5.6-luna",
    "fixer_backend": "pi",
    "fixer_model": "openai/gpt-5.6-luna",
    "executor_timeout": "45m",
    "auto_resume": "on"
  }
}
```

Every mutation except `trust` and `migrate` uses the same run, input, gate, token and action fields. `trust` and `migrate` need `action_id` and a `reason`; `trust` also needs the `digest` it trusts.

- `resolve-rules`, `review` and `cancel` need no settings and no reason.
- `apply` and `save` need a reason and a settings object with `mode`, `preset`, `limit` and `rules`. Omitted backend, model and `executor_timeout` fields keep their current values.
- `save` takes `"target":"user"` (the default) or `"target":"repository"`.
- `new` retains previous evidence and uses saved defaults.

The other mutations are described where they belong:

- `repair` and `handback`: [repair and handback](repair.md).
- `resume`, `rerun`, `rerun-from-start` and `cancel-auto-resume`: [pause, resume and rerun](agents.md#backend-pause-resume-and-rerun).
- `pipeline`, `resolve-conflict` and `decide`: [full pipeline](pipeline.md).
- `recover`: [recovery](recovery.md).
- `trust` and `migrate`: [configuration](configuration.md).

### Source movement

When the source branch or head moves, Apply and Save are blocked on the old run. A current, token-bound `new` request can capture the moved source when no writer or unknown-outcome hazard exists. Old evidence stays unchanged.

An unknown executor or writer outcome blocks replacement until an explicit [recovery](recovery.md) answers it.

### Replay

Replay an identical mutation with the same action ID to get its recorded result, even after a reconnect or restart. This works while the receipt is retained; see [storage limits](state.md#assumptions-and-limits). After pruning, inspect the current run before retrying.

- Reusing an action ID with different input is refused.
- A new action ID needs the current token.
- A recorded result may describe an older snapshot, so inspect the active run after a replay.

### Outcomes

An outcome of `ok` means the lifecycle request succeeded. It never means a validation stage passed. Errors and unsupported operations have the outcome `refused` or `error`.

## Settings

| Field | Default | Values |
| --- | --- | --- |
| `mode` | `auto` | `ask`, `auto`, `yolo`; see [modes and decisions](pipeline.md#modes-and-decisions) |
| `preset` | `one-pass` | `one-pass`, `full` |
| `limit` | 4 | 1 through 1000, the combined review and repair limit |
| `rules` | `unslop`, `no-comments` | at most 32 rule names; see [the rule contract](rules.md) |
| `reviewer_backend`, `fixer_backend` | the first installed of `pi`, `claude`, `codex`; `pi` when none is installed | `pi`, `claude`, `codex` |
| `reviewer_model`, `fixer_model` | native default | a model ID; Pi uses provider-qualified IDs |
| `executor_timeout` | `45m` | a positive Go duration |
| `auto_resume` | empty | `on`, `off`, or empty; see [automatic resume](agents.md#automatic-resume) |

The numeric limit is provisional. A reduction cannot erase consumed usage. Modes apply to the full preset. A one-pass repair always needs an explicit selection.

### Executor timeout

`executor_timeout` limits each reviewer or fixer executor, including its descendants.

- Use a positive Go duration string such as `30s`, `45m` or `1h`. Zero, negative, malformed and overflowing durations are refused.
- An omitted or empty value inherits the current setting. Older configurations fall back to `45m`.
- Apply the value to a run with `apply`, or save it as a user default with `save`. Trusted repository overrides may also set it.

Parent cancellation and earlier parent deadlines still stop execution. After a daemon interruption, the child supervisor uses the executor deadline plus 30 seconds for cleanup.

## Events

`watch` (version 2) waits for committed run changes in the invoking repository:

```json
{"version":2,"op":"watch","stream":"from the last answer","after":17,"wait_ms":25000}
```

Each answer carries the current snapshot of the worktree's run, as `inspect` returns it, plus `stream`, `sequence` and `wait`. `wait` is one of:

| `wait` | Meaning |
| --- | --- |
| `events` | `events` lists each change after `after`, with its `sequence`, `kind`, `run_id`, `stage`, `status` and `token`. Step events also carry `execution`, `command`, `exit_code`, `elapsed_ms` and `checks`. |
| `elapsed` | Nothing changed within `wait_ms` (0 to 60000; 0 answers at once). The outcome is `ok`. An elapsed wait cancels nothing and is not a failure; a review keeps running. |
| `resync` | The client's position is unusable. The client sent no `stream` or another one (for example after a daemon restart), a sequence ahead of the daemon, or one older than the retained 1,024 events. Treat the snapshot as authoritative and continue from its `stream` and `sequence`. |

Every version-2 `attach` and mutation response also carries `stream` and `sequence`: the last event the daemon committed before it handled the request. A replay returns the values recorded with its receipt.

An event with a higher sequence is newer than that response, so a client can drop a watch answer that arrives late. Pass `stream` and `sequence` from each answer into the next request. A client that loses its connection reconnects with the same values and receives either the missed events or a resync.

Watching never creates a run or authorizes a gate. At most 64 `watch`, `logs` follow and `pipeline` waits can be pending at once.

## Host mode

A host such as Posse can embed Greenhouse as its reviewer. Requests with `host` (version 2) create or act on a host-owned run that only reviews:

- The host owns author repair, candidate custody, publication, CI and cap decisions.
- Each `review` needs a host `grant`.
- `host_result` reports the exact-head verdict and findings, plus any `generated` and `uncovered` paths. A review with uncovered paths and no findings settles as `status: "partial-review"` with `verdict: "partial"`, never as clean. See [generated and uncovered paths](embedding.md#generated-and-uncovered-paths).

Other clients can only observe a host run. Another host can replace it with `attach` or `new` once the run is idle: not running, awaiting recovery, scheduled for automatic resume, or holding a candidate. See the [embedding contract](embedding.md).
