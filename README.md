<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/greenhouse-mark-dark.svg">
  <img src="docs/assets/logo/greenhouse-mark.svg" alt="Greenhouse logo" width="120">
</picture>

# Greenhouse

Greenhouse is a standalone terminal and JSON client for a per-user local run store. This slice implements lifecycle, settings, immutable rule resolution, and an explicitly configured independent Pi review. Review success is not test, lint, or delivery success.

## Build and open

```sh
go build -o greenhouse .
cd /path/to/a/git/worktree
/path/to/greenhouse
```

The executable requires Linux, Git, and a committed worktree. Runtime operation requires neither Go, Posse, nor Herdr. The bare command opens a single-pane, line-oriented terminal client. Press Enter after each command. `i` refreshes inspection, `c` toggles criteria details, `r` explicitly resolves rules, `s` opens Settings, and `n` requests New run. `p` prepares intent and an exact target commit, `v` dispatches one review, `x` cancels it, and `q` detaches. Ctrl-C also disconnects without cancelling the daemon.

Settings edits are drafts until you select Apply or Save. Apply adds an immutable run-local snapshot. Save changes user defaults without changing the current run. Use Settings twice to do both. Rule selection does not load bytes. Explicit resolution snapshots content; dispatch retains the exact emitted criteria and committed diff. The terminal supports mode, preset, combined review/repair limit, and rule selection. Repository overrides are unsupported and untrusted.

## JSON interface

Send one request on stdin to `greenhouse axi`. The invoking directory identifies the worktree; the CLI replaces any request `path` with that directory. Machine output is one JSON envelope on stdout.

```sh
printf '%s\n' '{"version":1,"op":"attach"}' | /path/to/greenhouse axi
```

Git identity commands discard inherited `GIT_*` variables so the daemon cannot reuse the first client's repository selection for later clients. The invoking path determines identity, including linked worktrees.

`attach` creates a run only when its canonical context has no active slot. `inspect` never creates a run. Reopening retained work does not consume budget, change snapshots, or execute a stage.

Copy `run_id`, `input_id`, `gate_id`, and `token` from the response into a mutation. Use `token` as `expected`:

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
    "rules": ["unslop", "no-comments"]
  }
}
```

Supported mutations are `apply`, `save`, `new`, `resolve-rules`, `prepare`, `review`, and `cancel`. All mutations use the same run, input, gate, token, and action fields. `resolve-rules`, `review`, and `cancel` require no settings or reason. `apply` and `save` require a complete settings object and a reason. `new` retains previous evidence and uses saved defaults. Source branch/head movement blocks Apply and Save on the old run. A current token-bound New request can capture the moved source when no writer or unknown-outcome hazard exists. Old evidence remains unchanged. Unknown executor or writer outcomes still block replacement; there is no recovery override in this slice.

Replay an identical mutation with the same action ID to obtain its recorded result, even after reconnect or restart. Reusing an ID with different input refuses. New action IDs require current tokens. Inspect the active run after replay: a recorded result may describe an older snapshot. An outcome of `ok` means the lifecycle request succeeded, never that a validation stage passed. Errors and unsupported operations have `refused` or `error` outcomes. There are no approval, acceptance, repair, publication, or recovery commands.

Defaults are `auto`, `one-pass`, a combined limit of 4, and selected `unslop` and `no-comments` names. The numeric limit is provisional. Valid limits are 1 through 1000; reductions cannot erase consumed usage. Modes and presets are stored selections, not active automation.

## Independent review configuration

Create a private `reviewer.json` in `GREENHOUSE_HOME` (or the default state directory), with mode 0600:

```json
{"executable":"/absolute/path/to/pi","model":"provider/exact-model-id"}
```

Only per-user configuration grants executable authority. There are no arbitrary arguments, repository overrides, or session selectors. Models require an exact provider-qualified ID without a thinking suffix or wildcard. A successful response must report the same provider and model. The adapter targets the installed Pi 1.5.0 CLI; incompatible flags or event output yield an inconclusive result.

Prepare (`p`, or AXI `prepare` with `intent` and `target`) captures a full commit object ID, the current candidate, selected committed diff, and reviewer configuration. Inspect `run.selected_diff` or toggle `c` before dispatch. Resolve rules, then request `review` with current mutation tokens. Review rejects changed reviewer configuration until another explicit Prepare. Dirty files are neither included nor changed. Empty diffs, binary patches, submodules, and inputs exceeding 1 MiB refuse; the encoded bundle is capped at 2 MiB.

Each run permits one invocation. Admission consumes review usage even if launch fails, the process is cancelled, or its output is inconclusive. Apply cannot reset usage. `full`, repairs, cap decisions, acceptance, forge operations, and automatic retries are unsupported. New run retains prior finding identities; omission does not resolve them. Resolution actions are not implemented.

Pi receives a fresh ephemeral session, explicit read-only tools, no extensions, MCP, skills, context files, prompt templates, or project approval. Its working directory and agent directory are fresh and private. Author session routing and process-injection environment variables are removed. The adapter uses environment-based provider credentials; existing Pi auth files and custom provider files are not copied or loaded. Read-only tools are not an OS sandbox. Inspection is patch-based; no complete source-tree snapshot is provided.

`run.review` exposes backend, requested and reported model, deterministic argv, configuration digest, exact emitted input and criteria digests, coverage, findings, status, and bounded execution summaries. Output is capped at 4 MiB, stderr at 64 KiB, and runtime at ten minutes. After child exit or cancellation, inherited output pipes close after a 250 ms grace period. Forced pipe closure without cancellation yields an inconclusive result. Descendants are not signalled by stored PID or process group. Raw transcripts and stderr are withheld to avoid persisting credentials. The terminal displays the same findings, provenance, and summary; `c` also shows the diff. A successful verdict certifies only the bound independent-review input. Config changes and source movement mark current evidence stale; historical verdicts remain inspectable.

## State and compatibility

`GREENHOUSE_HOME` overrides `~/.local/state/greenhouse`. The state directory must be private, with mode 0700. The Unix socket path must fit the platform's Unix-socket path limit; use a short state path in deeply nested environments.

One daemon owns an advisory `flock` before reading recovery state or binding the socket. It never unlinks a live listener. Requests execute serially. Each request commits the entire version-1 store by writing and syncing a same-directory temporary file, renaming it, and syncing the directory before responding. Action receipts and run changes share that commit. Startup parks persisted `running` or `writing` states as `recovery-required`; it never reruns them. Invalid run records and unknown store versions refuse startup. Rule snapshots are an additive version-1 extension: old stores need no rewrite or counter migration. New snapshots validate source, exact bytes, digest, parsed criteria, and configuration identity on load.

This design assumes a local filesystem with reliable flock, atomic rename, and fsync. It does not support network filesystems, concurrent external store editors, cross-user access, online migration, or mixed protocol versions. Disk corruption has no automatic repair. A directory-sync failure can leave an uncertain commit; inspect after restart instead of inventing another mutation. Back up the state directory while the daemon is stopped. Storage grows with retained runs, events, receipts, and snapshots; compaction is not implemented.

The daemon commits review admission and consumption before starting a child process. Review completion returns through the serialized controller; inspection remains available during execution. Cancellation addresses only the current child through its process handle. Restart parks unknown outcomes without signalling stored PIDs or retrying. To stop the daemon administratively, send SIGTERM to the PID recorded in the isolated state's `pid` file after verifying the process. Client exit is not daemon shutdown. All Git commands are observational; Greenhouse does not initialize, stash, reset, rebase, commit, modify refs, or create worktrees.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l *.go)"
go build -o "$TMPDIR/greenhouse" .
python3 tools/terminal_check.py "$TMPDIR/greenhouse" docs/evidence
```

Set `TMPDIR` to an isolated scratch directory. Integration fixtures use private state homes and temporary Git repositories, and build the executable with the race detector. The Python check drives actual 56- and 100-column PTYs and compares their Apply, Save, New run, and reattachment results with AXI. Logs are under the selected evidence directory. Restart fixtures seed synthetic usage and interrupted-writer state only while their daemon is stopped; they are not review evidence.

See [the rule contract](docs/rules.md) for selection, validation, provenance, and fixture delivery limits.

See [the implementation sequence](docs/implementation-plan.md) for the unsupported spec areas.
