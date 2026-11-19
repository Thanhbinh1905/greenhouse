# Repeat the first review by hand

The recorded sessions use scripted real PTYs. They were accepted as proof for this Task. A person has not repeated this checklist; it remains available for later acceptance.

Use a Git worktree with a committed change and an existing `main` target. Start with fresh Greenhouse state and no saved defaults.

- [ ] From the Greenhouse checkout, install Greenhouse and configure GitHub:

  ```sh
  mkdir -p "$HOME/.local/bin"
  GOBIN="$HOME/.local/bin" go install ./cmd/greenhouse
  export PATH="$HOME/.local/bin:$PATH"
  gh auth login
  gh auth setup-git
  ```

  Both authentication commands should succeed. Install and sign in to Pi, Claude Code or Codex with that CLI's own instructions. Greenhouse uses the agent's login, provider routing and default model, and needs no `reviewer.json` or `providers.json`; see [Install](../../../README.md#install).

- [ ] Open the committed change in the line client:

  ```sh
  cd /path/to/your/worktree
  git rev-parse main
  TERM=dumb greenhouse
  ```

  Copy the full target SHA before opening Greenhouse. `TERM=dumb` selects the line client, which the steps below use; for the interactive screen, follow [Run your first review](../../../README.md#run-your-first-review) instead. Expect `Greenhouse | independent review`, `Mode: auto | one-pass | limit 4`, and `Budget: review 0 + repair 0 = 0 of 4`. Rule entries should show `configured`.

- [ ] Type `p` and press Enter. Press Enter at `Reviewer agent` to keep the current agent, enter the intended behavior at `Intent:`, then paste the target SHA at `Review committed changes against Git target`. Type `l` and press Enter. Expect both selected rules to show `resolved`, with their digests. Type `c` to inspect the criteria and committed diff.

- [ ] Type `v` and press Enter. Type `i` to refresh until the review settles. Expect `clean-review`, `outstanding-findings`, or an explicit inconclusive or paused result. Findings must show priority, file, explanation, and ID. A completed review should consume one review round; this step must not push or create a PR.

- [ ] Type `q`, press Enter, then run `TERM=dumb greenhouse` again. Expect the same run ID, candidate, findings, and budget. Resize to 56, 80, and 100 columns and type `i` after each resize. Labels and prose should wrap between words; long identifiers may show `…`. Type `q` to detach.

The optional repair and full-delivery flows require the trusted configurations described in [repair](../../repair.md#configure-the-fixer) and [full pipeline](../../pipeline.md#command-plan). This checklist exercises the quick-start review without authorizing either flow.
