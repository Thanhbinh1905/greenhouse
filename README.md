<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/greenhouse-mark-dark.svg">
  <img src="docs/assets/logo/greenhouse-mark.svg" alt="Greenhouse logo" width="120">
</picture>

# Greenhouse

Asking your coding agent to review its own work is a bit like proofreading your own essay: it already knows what it meant, so it reads past the mistakes. Greenhouse gets your branch a second opinion. It starts a fresh agent that has never seen your conversation, hands it what you asked for and the committed diff, and lets it read the code. You get findings you can act on, and if you like, a fix for the ones you pick.

It runs on the Pi, Claude Code or Codex login you already have, so there are no keys to copy.

Here is what a review looks like in the terminal (from our [recorded UI check](docs/evidence/tui/walkthrough-findings-repair-80.txt), with a test reviewer):

```text
Greenhouse · feature @ 9764a62                           one-pass · auto · live
──────────────────────────────────────────────────────────────────────────────
 Findings · 2 outstanding findings · 0 selected

 ▸ [ ] P1 sample.txt:1 open · Sample text is wrong for the stated intent
   [ ] P2 sample.txt:1 open · Sample text gives no context for readers
 ─────────────────────────────────────────────────────────────────────────────
P1 correctness · sample.txt:1
Sample text is wrong for the stated intent
Action: Rewrite the line to say repaired
Reported for head 9764a62

↑↓ move  ⏎ details  space select  r repair selected  esc back  q quit
```

## Try it

You need Linux, Git and Go 1.24.2 or later, plus one of Pi, Claude Code or Codex installed and signed in.

```sh
go build -o greenhouse ./cmd/greenhouse
cd /path/to/your/feature-branch
/path/to/greenhouse
```

Commit changes first. Greenhouse reviews committed changes. On launch, it starts the first review without asking for setup.

Greenhouse infers intent from an agent session in this worktree, the open pull request body, or commit messages and the branch name, in that order. The source appears on screen. Press `e` to edit the intent before your next run.

The target uses the upstream branch or the default branch: `origin/HEAD`, `main` or `master`. Greenhouse loads built-in rules and uses the first installed agent's default model. If the branch has no commits or no agent is installed, Greenhouse says what to do. See the [terminal guide](docs/terminal.md) for controls.

## What happens next

Once the findings are in, you decide what to do with them. Tell Greenhouse which checks to trust in a small [`fixer.json`](docs/repair.md#configure-the-fixer), then select the findings worth fixing and press `r`. A fixer works on them in a private worktree, your own checks run against the result, and only when they pass can you hand the commit back to your branch. The fixer is held to the findings you selected.

If you want the whole trip, the [full pipeline](docs/pipeline.md) goes further: review, repair, tests, docs, lint, push, a PR, and a wait for your required CI checks. It stops at a green PR and leaves the merge to you.

Work doesn't stop when you close the terminal. A small local daemon keeps your runs, so you can press `q`, go do something else, and run `greenhouse` again to pick up where you left off. If Greenhouse is interrupted mid-repair, it shows you what it found and lets you choose how to [recover](docs/recovery.md).

## Where to go from here

| If you want to... | Read |
| --- | --- |
| Learn the keys, or use it from a pipe or `TERM=dumb` | [Terminal](docs/terminal.md) |
| Repair findings and hand the fix back | [Repair and handback](docs/repair.md) |
| Run review through to a green PR | [Full pipeline](docs/pipeline.md) |
| Script it with JSON (`greenhouse axi`) | [JSON interface](docs/axi.md) |
| Drive a full run from another program | [Worker contract v1](docs/worker-contract-v1.md) |
| Embed it as the reviewer in a tool like Posse | [Embedding contract](docs/embedding.md) |
| Switch agents, or deal with quotas and pauses | [Agents](docs/agents.md) |
| Set up config files or trust a repository's settings | [Configuration](docs/configuration.md) |
| Understand what happens after a crash | [Recovery](docs/recovery.md) |
| Find where runs and logs are stored | [State and daemon](docs/state.md) |
| Write or select review rules | [Rule contract](docs/rules.md) |
| Hack on Greenhouse itself | [Development](docs/development.md) |

There is also a [compatibility policy](docs/compatibility.md) and a page on [v1 status and known limits](docs/implementation-plan.md).

## Good to know

On Linux, agents run inside a Landlock sandbox that keeps them from writing to your repository or anywhere outside the directories Greenhouse gives them. It doesn't block outbound network access, since agents need to reach their providers, so Greenhouse records the confinement as `degraded`. If the kernel has no Landlock at all, the agent runs unconfined and the run's record says so. The [confinement notes](docs/agents.md#os-confinement) have the details.

A clean review is one useful signal, not a replacement for your tests, lint or CI.

The interactive screen is built with [Bubble Tea, Bubbles and Lip Gloss](THIRD_PARTY_NOTICES.md).
