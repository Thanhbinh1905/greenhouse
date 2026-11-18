# Drive a full run without a terminal

Worker contract version 1 uses JSON protocol version 2. Additive response fields
are compatible. Hosts must ignore unknown response fields. `greenhouse axi`
still reads one JSON request from stdin and writes one response. Each subcommand
prints one compact JSON response and never prompts.

## Start the current branch

Run these commands inside a committed Git worktree. Configure the native reviewer,
fixer, and trusted pipeline commands first, as described in
[the README](../README.md). A full delivery needs a remote,
GitHub owner and repository, PR base, and required check names in `pipeline.json`.
Greenhouse uses a private candidate branch and never merges the PR.

```sh
greenhouse axi pipeline --intent 'Describe the intended change' --target main --wait 8m
```

The command attaches the current worktree when no run exists. `pipeline` with an
intent prepares a new full run and resolves its rules. Omit `--target` to use the
repository's default target resolution. The intent becomes immutable after
preparation. An existing prepared run continues with:

```sh
greenhouse axi pipeline --wait 8m
```

The response has `outcome`, `run_id`, `input_id`, `gate_id`, `token`, and `run`.
A successful request has `outcome: "ok"`; a rejected request has
`outcome: "refused"` and `error`. A transport or flag error exits nonzero.
A refused protocol operation still exits zero; scripts must check `outcome`.

`pipeline` waits until a decision, park, `locally-ready`, `ci-ready`, or another
terminal outcome. Cancellation waits for executor cleanup to settle.
`wait: "elapsed"` returns the running snapshot when its wait
expires. Expiry does not cancel the run. `--wait` accepts a Go duration, up to
24 hours. `--wait_ms` accepts the same bound in milliseconds. Without a wait,
the operation returns after admission.

The raw equivalent after attach supplies the current identities:

```json
{"version":2,"op":"pipeline","run_id":"RUN","input_id":"INPUT","gate_id":"GATE","expected":"TOKEN","action_id":"UNIQUE","intent":"Describe the intended change","target":"main","wait_ms":480000}
```

Reuse an `action_id` only for an exact retry of the same mutation request.
Raw hosts call `attach` first and copy the returned identities. Subcommands
resolve omitted identities through `inspect` before submitting the same op.
They never retry a stale mutation automatically.

## Wait for progress

```sh
greenhouse axi watch
# Copy stream and sequence from this response, then long-poll:
greenhouse axi watch --stream STREAM --after SEQUENCE --wait 1m
```

The raw requests are:

```json
{"version":2,"op":"watch"}
{"version":2,"op":"watch","stream":"STREAM","after":42,"wait_ms":60000}
```

Every response contains the authoritative snapshot. Save its `stream` and
`sequence` for the next request. `wait` is `events`, `elapsed`, or `resync`.
`resync` means the daemon restarted or the bounded event history no longer
contains the cursor. Accept that snapshot and resume with its cursor.
A fresh `watch --wait 1m` obtains a cursor and waits once. An explicit cursor
allows scripts to retain events between invocations.

`watch` and log follow accept `wait_ms` from 0 through 60000. Repeat long-polls
with the returned cursor; an elapsed wait needs no delay or busy polling.
`--run` or `--run_id` selects a historical run in the same repository.

Events have `sequence`, `run_id`, `stage`, `status`, and `token`.
`kind: "decision"` announces an open decision. `kind: "park"` announces a
paused run or a stop requiring attention. Read `run.pause`, `run.recovery`,
`decision`, and `blocked` to distinguish these cases. `run-changed` covers
other state transitions. CI observations emit `kind: "checks"` with a `checks`
array in the same format as `delivery.checks`.

A shell script can follow the current run with `jq`:

```sh
reply=$(greenhouse axi pipeline --intent 'Describe the intended change' --target main --wait 8m)
while [ "$(printf '%s' "$reply" | jq -r '.outcome')" = ok ] &&
      [ "$(printf '%s' "$reply" | jq -r '.run.status')" = running ]; do
  stream=$(printf '%s' "$reply" | jq -r '.stream')
  after=$(printf '%s' "$reply" | jq -r '.sequence // 0')
  reply=$(greenhouse axi watch --stream "$stream" --after "$after" --wait 1m)
done
printf '%s\n' "$reply" | jq '{outcome, error, decision, delivery, status: .run.status}'
```

`step-started`, `step-finished`, and `step-failed` name an `execution` and the
redacted `command` argument array. Completion events include `exit_code` and
`elapsed_ms`. PR and CI operations use `greenhouse pr` and `greenhouse ci`
as command labels; their exit codes are logical success or failure (0 or 1).
Process events carry the actual child exit code. An executor error emits
`step-failed` even when that exit code is zero. A step failure can precede an
authorized repair or a transport retry; use the run snapshot for the outcome.

## Read and answer a gate

```sh
greenhouse axi inspect
```

An open `decision` contains `id`, `kind`, `stage`, `options`, `reason`, and
`findings` (finding IDs). Join those IDs with `run.findings`. Each finding has
`id`, `priority`, `file`, `start`, `end`, `explanation`, and `action`; `start`
and `end` are inclusive line numbers. The top-level `gate_id` identifies the
current gate, and `token` is its answer token.

For a findings gate offering `repair`:

```sh
greenhouse axi decide repair --reason 'Repair the reported defect'
```

Optional `--selected ID` flags select findings from the offered group.
Only choose an option in `decision.options`. Some decisions require User
authority, shown by `user_only`; a host must obtain that authority before
answering. A reason is required. The raw equivalent is:

```json
{"version":2,"op":"decide","run_id":"RUN","input_id":"INPUT","gate_id":"GATE","expected":"TOKEN","action_id":"UNIQUE","choice":"repair","reason":"Repair the reported defect"}
```

The CLI resolves the current run, input, gate, and token automatically.
Explicit `--run_id`, `--input_id`, `--gate_id`, `--expected`, and `--action_id`
flags preserve a script's inspected identities. A changed gate or token then
returns `outcome: "refused"` with `error: "stale run/input/gate/token"`.
After a decision starts work, follow `watch` until the next stop.

## Read output through Greenhouse

```sh
greenhouse axi logs --step test
greenhouse axi logs --step test --full
greenhouse axi logs --step test --offset 0 --follow --wait 1m
```

Steps are `review`, `repair`, `sync`, `test`, `docs`, `lint`, `push`, `pr`, and
`ci`. The default reads the last 16 KiB. `--full` returns the retained output.
An explicit `--offset 0` starts at the beginning.
For incremental reads, use `--follow` and pass `logs.next` as the next `--offset`:

```json
{"version":2,"op":"logs","step":"test","offset":0,"follow":true,"wait_ms":60000}
```

`logs` contains `step`, `offset`, `next`, `bytes`, `limit`, `content`, and
`truncated`. Offsets count bytes in the redacted stream. A normal incremental
response contains at most 64 KiB. Each run retains at most 4 MiB per step,
including repeated attempts. `truncated` signals that the limit was reached.
A negative offset or an offset beyond `bytes` is refused.

Follow waits for output when the selected step is active. It returns immediately
if the step is inactive or the run has stopped. `wait` is `output`, `complete`,
or `elapsed` after a parked follow request. Advance with `next`; use `watch`
to wait for a future step to start.

Process output is redacted before storage, using the transcript's environment
secret names and token patterns. Complete lines become readable while a process
runs. An unfinished line is withheld until a newline or process completion;
lines larger than 4 MiB are omitted. Command headers, exit summaries, stage
results, and CI observations share the same stream. PR and CI use structured
operation results because they run through the forge API.

```sh
greenhouse axi transcript
```

`transcript.content` keeps the existing bounded reviewer and fixer transcript.
`transcript.steps` contains the stage log tails used by the terminal's transcript
page. The terminal and AXI therefore read the same redacted stage output.

## Verify delivery before accepting done

```sh
greenhouse axi inspect --run RUN
```

Require `run.status: "ci-ready"` for delivery. `locally-ready` means the trusted
plan selected local-only execution. Inspect `run.pipeline.stages` for stage
results, commands, and failure reasons.

The top-level `delivery` contains:

- `pushed_head`: the candidate confirmed by the push stage.
- `pr_number` and `pr_url`: the persisted forge identity.
- `checks`: one entry per required check, with `name`, `status`, `conclusion`,
  and `head`. A missing observation has status `unobserved`.
- `current`: whether the source identity and private candidate still match the
  run, and the pushed head, PR head, and passed CI evidence name that candidate.

Check `delivery.current`, compare `delivery.pushed_head` with `run.candidate`,
and require every required check to have `status: "completed"`,
`conclusion: "success"`, and `head` equal to that candidate.
`current` validates local identities against the recorded delivery evidence;
it does not query the forge again after completion. A host requiring a fresh
remote observation must verify the PR head and checks with the forge.

To cancel active work:

```sh
greenhouse axi cancel
```

Cancellation returns after admission. Follow `watch` until cleanup settles;
`recovery-required` means Greenhouse could not prove the writer stopped.
