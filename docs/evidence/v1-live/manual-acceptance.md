# Repeat the first review by hand

The recorded sessions use scripted real PTYs. They were accepted as proof for this Task. A person has not repeated this checklist; it remains available for later acceptance.

Use a Git worktree with a committed change and an existing `main` target. Start with fresh Greenhouse state and no saved defaults.

- [ ] From the Greenhouse checkout, install and configure GitHub:

  ```sh
  mkdir -p "$HOME/.local/bin"
  GOBIN="$HOME/.local/bin" go install .
  export PATH="$HOME/.local/bin:$PATH"
  gh auth login
  gh auth setup-git
  ```

  Both authentication commands should succeed. Create the private `reviewer.json` using the exact Python block in [First review](../../../README.md#first-review). Configure `providers.json` there if your Pi model uses a custom endpoint.

- [ ] Load your existing Pi key and open the committed change:

  ```sh
  export OPENAI_API_KEY="$(pi auth print-api-key --provider openai)"
  cd /path/to/your/worktree
  git rev-parse main
  greenhouse
  ```

  Copy the full target SHA before opening Greenhouse. Expect `Greenhouse | independent review`, `Mode: auto | one-pass | limit 4`, and `Budget: review 0 + repair 0 = 0 of 4`. Rule entries should show `configured`.

- [ ] Type `p`, press Enter, enter the intended behavior, then paste the target SHA at `Exact target commit ID:`. Type `l` and press Enter. Expect both selected rules to show `resolved`, with their digests. Type `c` to inspect the criteria and committed diff.

- [ ] Type `v` and press Enter. Type `i` to refresh until the review settles. Expect `clean-review`, `outstanding-findings`, or an explicit inconclusive or paused result. Findings must show priority, file, explanation, and ID. A completed review should consume one review round; this step must not push or create a PR.

- [ ] Type `q`, press Enter, then run `greenhouse` again. Expect the same run ID, candidate, findings, and budget. Resize to 56, 80, and 100 columns and type `i` after each resize. Labels and prose should wrap between words; long identifiers may show `…`. Type `q` to detach.

The optional repair and full-delivery flows require the trusted configurations described in [the run report](README.md). This checklist exercises the quick-start review without authorizing either flow.
