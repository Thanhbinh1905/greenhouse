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

1. `attach` with `host`, then `apply` the rules and reviewer model, `prepare` the intent and target branch or commit, and `resolve-rules`.
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
  "model": "openai/gpt-5.6-luna"
}
```

`verdict` is `clean`, `findings` or `inconclusive`, and is empty until the review settles. `current` is false once the worktree branch or head moved, or the review went stale; a host must not record a verdict whose `current` is false as evidence for any head. Every finding carries the head it was reviewed on. Translate findings by `id`; an ID is stable across rounds for the same root cause.

A backend pause, such as an exhausted quota, keeps the review's grant and gives no verdict. `resume` or `rerun` continues that review under the same grant, so the host spends no second grant. Interrupted reviews use [recovery](../README.md#recovery-after-interruption) as in standalone mode.

## After an author repair

The host's writer commits the repair in the worktree. The run then shows the moved source and `current: false`. The host calls `new` with `host` to open a successor for the new head. The successor:

- links `predecessor` and the old run's `successor`;
- keeps `review_used` and `repair_used` and records them in `inherited`, so usage never resets;
- keeps the used grants, so a spent grant cannot buy another review.

Then the host repeats the review round with a new grant.

## Verification

`TestEmbeddedHostOwnsDelivery` drives a fake host through the daemon, AXI and terminal with an isolated HOME, real Git and a fake reviewer. It proves two grants buy exactly two reviewer invocations and no fixer, a Greenhouse `limit` of 1 does not stop the host's second round, the successor keeps usage, and no candidate ref, private worktree, lease or remote ref appears. `TestHostOwnershipBoundaries` covers standalone takeover and a second host, and `TestHostReviewResumesUnderItsGrant` covers a paused review that resumes under its grant.
