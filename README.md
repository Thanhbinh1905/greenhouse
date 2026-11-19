<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo/greenhouse-mark-dark.svg">
  <img src="docs/assets/logo/greenhouse-mark.svg" alt="Greenhouse logo" width="120">
</picture>

# Greenhouse

Greenhouse runs an independent code review of your branch with Pi, Claude Code or Codex. It can repair the findings you select, run your checks, and deliver the result as a GitHub PR whose required checks pass. It never merges.

Each agent runs in a fresh session under OS confinement. A local daemon keeps every run, so you can detach, reattach and recover after a crash.

A passing review is not a passing test, lint or delivery.

## Install

You need:

- Linux and Git.
- Pi, Claude Code or Codex, installed and signed in with that CLI's own instructions. Greenhouse uses the agent's login, provider routing and default model.
- Go 1.24.2 or later, to build.

Build the executable:

```sh
go build -o greenhouse ./cmd/greenhouse
```

Running Greenhouse needs neither Go, Posse nor Herdr.

## Run your first review

1. Commit your changes on a feature branch. Greenhouse reviews committed changes only and excludes dirty files.
2. Run Greenhouse inside that worktree:

   ```sh
   cd /path/to/a/git/worktree
   /path/to/greenhouse
   ```

   The screen shows the branch, the agent it will use, the suggested Git target and one highlighted action, **Run review**.
3. Press Enter. Run collects the intent, saves it with the target, loads the selected rules and starts one independent review.
4. Read the findings. Press `f` to open them.

Run reads the intent from your own agent history: Claude Code, Codex and Pi sessions that started in this worktree and mention the changed files.

- Run keeps your requests word for word, redacts secrets and drops tool output. It labels the intent with the agent, session and time.
- The intent is never a summary and never comes from the diff.
- When no session matches, Run asks what the change should do. When several sessions match equally, it asks which one.
- When the requests exceed the 8192-byte intent limit, Run asks for a shorter intent. It never cuts them.

To set the intent yourself, Tab to the Intent row and type it before Run. Tab to Agent or Target to change them. Changing the agent changes the reviewer only.

Each step of Run is a separate controller action. A refused or failed step stops Run and says why. Run never retries it.

Opening, resizing, navigating and detaching run nothing and use no budget. Press `q` to detach; the work keeps running in the daemon. Run `greenhouse` again to reattach.

The first review needs no `reviewer.json`, `providers.json` or `backends.json`, no copied key and no executable path. When the agent is not signed in, Greenhouse names the agent's own login action. Greenhouse never runs a login or changes credential files.

## Ways to use Greenhouse

| Use | Summary | Guide |
| --- | --- | --- |
| Terminal UI | The interactive screen and the line client for pipes and `TERM=dumb`. | [Terminal](docs/terminal.md) |
| JSON interface | `greenhouse axi` takes one JSON request on stdin and answers with one JSON envelope. | [JSON interface](docs/axi.md) |
| Selected repair | A fixer repairs the findings you select, trusted checks run, and handback fast-forwards your branch. | [Repair and handback](docs/repair.md) |
| Full pipeline | Review, repair, test, docs, lint, push, PR and CI, ending at `ci-ready`. | [Full pipeline](docs/pipeline.md) |
| Without a terminal | AXI subcommands drive a full run for a scripted worker. | [Worker contract v1](docs/worker-contract-v1.md) |
| Host mode | A host such as Posse embeds Greenhouse as its reviewer. | [Embedding contract](docs/embedding.md) |

Selected repair needs trusted checks in `fixer.json`. The full pipeline also needs a trusted command plan in `pipeline.json` and GitHub authorization. These files approve commands, not providers.

## Documentation

| Topic | Document |
| --- | --- |
| Keys, screens, line client and Settings | [docs/terminal.md](docs/terminal.md) |
| AXI requests, mutations, replay, events and host mode | [docs/axi.md](docs/axi.md) |
| One-pass repair, `fixer.json`, handback and resolution responses | [docs/repair.md](docs/repair.md) |
| Agent backends, review snapshot, OS confinement, pause, resume and rerun | [docs/agents.md](docs/agents.md) |
| Full pipeline stages, modes, decisions and authority grants | [docs/pipeline.md](docs/pipeline.md) |
| Recovery after interruption, candidate custody and leases | [docs/recovery.md](docs/recovery.md) |
| Configuration versions, migration, repository overrides and trust | [docs/configuration.md](docs/configuration.md) |
| State directory, daemon, transcript logs and storage limits | [docs/state.md](docs/state.md) |
| Rule selection, validation, provenance and fixture delivery limits | [docs/rules.md](docs/rules.md) |
| Protocol and configuration compatibility | [docs/compatibility.md](docs/compatibility.md) |
| Verification commands and model evaluations | [docs/development.md](docs/development.md) |
| v1 status and known limits | [docs/implementation-plan.md](docs/implementation-plan.md) |

The interactive screen uses [Bubble Tea, Bubbles and Lip Gloss](THIRD_PARTY_NOTICES.md).
