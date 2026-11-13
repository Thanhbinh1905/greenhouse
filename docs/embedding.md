# Embedding contract

A host such as Posse can use Greenhouse as its independent reviewer without running a second delivery pipeline. In host mode Greenhouse runs reviews and reports exact-head verdicts. The host keeps every other responsibility.

Host mode needs protocol version 2. See [compatibility](compatibility.md).

## Responsibilities

| Responsibility | Owner in host mode | Greenhouse behavior |
| --- | --- | --- |
| Independent review of the exact head | Greenhouse | Runs one fresh-context review per host grant |
| Rule loading and snapshots | Greenhouse | Same as standalone: `apply`, `resolve-rules` |
| Author repair and conflict repair | Host | Refuses `repair` and `resolve-conflict` |
| Candidate custody | Host | Creates no private worktree, candidate ref or lease; refuses `handback` |
| Push, PR and CI | Host | Refuses `pipeline` |
| Review cap and cap decisions | Host | Ignores the Greenhouse `limit`, `mode` and `preset`; refuses `decide` and `rerun-from-start`; never reports budget exhaustion |
| Saved configuration | User | Refuses `save`, `trust` and `migrate` from the host |

Each responsibility has one owner. Greenhouse never pushes, opens a PR, writes the worktree, or creates a reviewer pane in host mode.

## Host identity

Every host request carries `host`:

```json
{"name": "posse", "id": "greenhouse/t69", "metadata": {"worker": "w-3"}}
```

`name` and `id` are required text. `metadata` holds at most 16 string pairs and is stored as given. Greenhouse does not interpret the identity; it does not need a Project, Task or Herdr.

- `attach` with `host` creates a host run for the worktree, or attaches to the run of the same host (same `name` and `id`).
- A worktree with a standalone run refuses a host `attach`. The host takes the worktree over only with an explicit `new` carrying `host`.
- A run owned by one host refuses mutations from any other client, including the terminal and AXI requests without `host` or with a different host. Those clients may still `attach`, `inspect` and `watch`; their `actions` list only `attach` and `inspect`, and `blocked` names the owning host.

## Review rounds

1. `attach` with `host`, then `apply` the rules and reviewer model, `resolve-rules`, and `prepare` the intent and target branch or commit.
2. `review` with `grant` set to the host's identity for this review authorization, for example its review round ID. Greenhouse refuses a review without a grant and refuses a grant this run or its predecessors already used. Replaying the same `action_id` returns the recorded receipt.
3. Follow the run with `watch`, then read `host_result`.

`prepare` resolves the requested target to a commit and stores it as `run.target`. The review diff uses `git merge-base <target> <candidate>`, reported as `run.review_base` in `inspect` and `review_base` in `host_result`. Pipeline sync and PR creation keep using `target`. If the histories are unrelated, the diff uses `target` and `review_base_reason` explains the fallback.

`host_result` is present once a host run has a review:

```json
{
  "host": {"name": "posse", "id": "greenhouse/t69"},
  "grant": "round-1",
  "review_id": "...",
  "head": "exact reviewed commit",
  "target": "exact target commit",
  "review_base": "merge-base of target and head",
  "status": "outstanding-findings",
  "verdict": "findings",
  "current": true,
  "findings": [{"id": "...", "priority": "P2", "origin": "policy", "file": "a.go", "start": 3, "end": 4, "explanation": "...", "action": "...", "rule": "unslop/plain-prose", "head": "exact reviewed commit"}],
  "rules_digest": "...",
  "config_digest": "...",
  "input_digest": "...",
  "backend": "pi",
  "model": "openai/gpt-5.6-luna",
  "reason": "",
  "diagnostic": "",
  "usage": {
    "input_tokens": 100,
    "output_tokens": 20,
    "cached_input_tokens": 0,
    "tokens": 120,
    "cost_usd": 0.01
  }
}
```

`verdict` is `clean`, `findings` or `inconclusive`, and is empty until the review settles. `current` is false once the worktree branch or head moved, or the review went stale; a host must not record a verdict whose `current` is false as evidence for any head. Every finding carries the head it was reviewed on. Translate findings by `id`; an ID is stable across rounds for the same root cause.

Failed or inconclusive reviews include a classified `reason` and a credential-redacted `diagnostic` excerpt, bounded to the last 40 lines and at most 4 KiB, with `[truncated]` when shortened. These fields are omitted when empty. `usage` reports input, output, cached-input, and total tokens, plus cost in USD. Unknown values stay zero; totals retain each backend's accounting.

A backend pause, such as an exhausted quota, keeps the review's grant and gives no verdict. `resume` or `rerun` continues that review under the same grant, so the host spends no second grant. Interrupted reviews use [recovery](../README.md#recovery-after-interruption) as in standalone mode.

## Request examples

Each request block below is a complete JSON request to `greenhouse axi`, invoked from the candidate worktree. AXI reads one request per invocation. It writes one response envelope on stdout and replaces any request `path` with the invoking directory.

The response blocks show selected fields and collection entries; other response data is omitted. Angle-bracket values are placeholders. Each mutation copies `run_id`, `input_id`, and `gate_id` from the latest response, and copies its `token` into `expected`. An `outcome` of `ok` confirms the operation, not a clean review. A refused or failed operation stops the sequence.

### Attach or start a successor

`attach` creates a host run if the worktree has no active run, or reattaches to a run with the same host identity:

```json
{
  "version": 2,
  "op": "attach",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  }
}
```

Response fields:

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<token>",
  "run": {
    "host": {
      "name": "posse",
      "id": "greenhouse/t69"
    }
  }
}
```

After the host commits an author repair, `new` captures the changed head. It also allows explicit takeover of a standalone run. Copy the current envelope fields from `attach` or `inspect`:

```json
{
  "version": 2,
  "op": "new",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-new-2"
}
```

The response has a new `run_id`, `input_id`, and `token`; `run.predecessor` identifies the previous run. The host repeats the remaining operations for the successor with new action IDs.

### Apply settings

`apply` selects rules and the reviewer without saving user defaults. It requires a complete settings object and a reason. An empty model selects the native agent's default model. Host mode ignores the standalone budget settings; they remain required settings fields.

```json
{
  "version": 2,
  "op": "apply",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-apply-1",
  "reason": "Review the host candidate with the selected rules",
  "settings": {
    "mode": "ask",
    "preset": "one-pass",
    "limit": 3,
    "rules": [
      "unslop",
      "no-comments"
    ],
    "reviewer_backend": "pi",
    "reviewer_model": "",
    "fixer_backend": "pi",
    "fixer_model": ""
  }
}
```

The response includes the refreshed envelope fields and a new entry in `run.configs` containing the applied `settings`. Applying rules selects their names; it does not load their bytes.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "run": {
    "configs": [
      {
        "id": "<snapshot>",
        "settings": {
          "mode": "ask",
          "preset": "one-pass",
          "limit": 3,
          "rules": [
            "unslop",
            "no-comments"
          ],
          "reviewer_backend": "pi",
          "reviewer_model": "",
          "fixer_backend": "pi",
          "fixer_model": ""
        }
      }
    ]
  }
}
```

### Resolve rules

`resolve-rules` loads and snapshots the selected rule bytes:

```json
{
  "version": 2,
  "op": "resolve-rules",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-rules-1"
}
```

The response includes the refreshed envelope fields, `rules`, `rules_ready`, and `run.rule_snapshots`. `rules_ready: true` means the selected rules loaded successfully; inspect rule errors before preparing a review.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "rules_ready": true,
  "rules": [
    {
      "selection": "unslop"
    },
    {
      "selection": "no-comments"
    }
  ],
  "run": {
    "rule_snapshots": [
      {
        "id": "<snapshot>"
      }
    ]
  }
}
```

### Prepare the review input

`intent` states what the change should do. `target` accepts a branch name or commit available in the local repository. The example uses a full, exact commit ID; replace the SHA with your target commit. Greenhouse records the resolved commit and reviews the diff from its merge-base with the candidate to the candidate. The recorded target remains that exact commit, even when the merge-base differs.

```json
{
  "version": 2,
  "op": "prepare",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-prepare-1",
  "intent": "Reject invalid request fields before starting a review",
  "target": "0123456789abcdef0123456789abcdef01234567"
}
```

The response includes the refreshed envelope fields and the retained `run.intent`, `run.target`, and `run.candidate`.

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<new token>",
  "run": {
    "intent": "Reject invalid request fields before starting a review",
    "target": "0123456789abcdef0123456789abcdef01234567",
    "candidate": "<candidate commit>"
  }
}
```

### Dispatch one review

`grant` identifies the host's authorization for this round. Each new round needs an unused grant, including rounds on a successor. Replaying the identical request with the same `action_id` returns its receipt; it does not spend another grant.

```json
{
  "version": 2,
  "op": "review",
  "host": {
    "name": "posse",
    "id": "greenhouse/t69"
  },
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "expected": "<token>",
  "action_id": "t69-review-1",
  "grant": "round-1"
}
```

Response fields at dispatch:

```json
{
  "version": 2,
  "outcome": "ok",
  "run_id": "<run>",
  "input_id": "<input>",
  "gate_id": "<gate>",
  "token": "<token>",
  "host_result": {
    "grant": "round-1",
    "head": "<candidate commit>",
    "target": "<target commit>",
    "status": "running",
    "current": true
  }
}
```

`watch` follows progress; `attach` or `inspect` reads the latest envelope. The settled `host_result` has the shape shown in [review rounds](#review-rounds). Accept a verdict only while `current` is true and `head` matches the host's candidate commit.

## After an author repair

The host's writer commits the repair in the worktree. The run then shows the moved source and `current: false`. The host calls `new` with `host` to open a successor for the new head. The successor:

- links `predecessor` and the old run's `successor`;
- keeps `review_used` and `repair_used` and records them in `inherited`, so usage never resets;
- keeps the used grants, so a spent grant cannot buy another review.

Then the host repeats the review round with a new grant.

## Verification

`TestEmbeddedHostOwnsDelivery` drives a fake host through the daemon, AXI and terminal with an isolated HOME, real Git and a fake reviewer. It proves two grants buy exactly two reviewer invocations and no fixer, a Greenhouse `limit` of 1 does not stop the host's second round, the successor keeps usage, and no candidate ref, private worktree, lease or remote ref appears. `TestHostOwnershipBoundaries` covers standalone takeover and a second host, and `TestHostReviewResumesUnderItsGrant` covers a paused review that resumes under its grant.

## Stop the daemon

Run `greenhouse stop` with the daemon's `GREENHOUSE_HOME`, or send an AXI request:

```sh
printf '%s\n' '{"version":2,"op":"stop"}' | greenhouse axi
```

This administrative operation applies to the entire state home and needs no run token, host identity, or grant.
The daemon refuses with `outcome: "refused"` and `error: "daemon is busy; let running work settle before stopping"` while work runs.
Let work settle and retry. A refusal leaves running children untouched.
On success, `outcome` is `ok` and `explanation` is `daemon stopped`.
If no daemon is running, `explanation` is `daemon is not running`.
Neither form starts a daemon. The client waits up to five seconds for shutdown to release the state-home lock.
Shutdown removes the `pid` file. Restart never signals stored PIDs; SIGTERM handling remains available.
