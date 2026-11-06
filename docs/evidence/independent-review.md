# Independent review verification

## Implemented boundary

The standalone controller admits one explicit Pi invocation per run. Prepare binds intent, a full target commit ID, selected committed diff, and trusted per-user executable/model configuration. Dispatch binds resolved rule bytes and consumes usage before launch. The reviewer has an ephemeral session, private working and agent directories, read-only tools, and disabled project resources. No author transcript, auth file, custom provider file, or project instructions are loaded automatically.

The finding ID hashes origin, path, range, rule, and normalized explanation. New runs inherit outstanding findings. Omission never resolves them. This is deterministic normalization, not semantic root-cause matching. Review is patch-based, not a complete source-tree assessment. Read-only tools are not an OS sandbox.

## Executable evidence

- Initial red: `go test ./... -run TestReviewRequiresExplicitIntent -count=1` failed because AXI returned `unsupported operation: review` instead of checking review prerequisites.
- Strict-output red: `go test -run TestVerdictRejects -count=1` accepted duplicate `verdict` fields before validation was added.
- Green: executable fake-Pi fixtures cover concurrent identical admission, stable replay, one-pass refusal, required-rule refusal, cap refusal, retained usage after an increase, dirty-file/ref preservation, finding persistence after restart, cancellation, malformed output, source movement, and a killed daemon with unknown executor outcome.
- Fixtures capture argv and stdin at the actual process boundary. They verify read-only tools, context suppression, exact rule text, and neutralized author routing. Review output requires complete path coverage, exact head and reported model, consistent verdict/findings, valid rule origins, and changed-hunk ranges.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and `gofmt` checks pass.
- `tools/terminal_check.py` drives real 56- and 100-column PTYs through Settings, Apply, Save, New, rule resolution/failure, fake-backed review launch, finding inspection, and one-pass refusal. AXI confirms the same durable state. Logs remain in Task scratch, not the repository.

## Review round 1 corrections

- The executable space-path regression failed with `finding range outside changed hunks` for line 3 in `has space.txt`. Git appends a tab delimiter to this unquoted patch header. The parser now removes that delimiter before decoding paths; filename spaces and quoted filename tabs remain intact. The same CLI fixture now retains the finding and its consumed invocation.
- The executable inherited-pipe regression failed with `status=cancelled elapsed=5.275639465s`. The reviewer had spawned a five-second descendant holding stdout and stderr. `exec.Cmd.WaitDelay` now bounds inherited-pipe waiting to a 250 ms grace period after exit or cancellation. CLI fixtures require cancellation and normal-exit completion within three seconds, before the descendant exits. Normal exit with forced pipe closure remains inconclusive; usage remains consumed. Fixture cleanup waits for the descendant without signalling unverified PIDs.

## Limits

No paid model or live provider compliance evaluation ran. The adapter targets Pi 1.5.0 flags and JSON events observed locally. Credentials must already be available through provider environment variables. Raw transcripts and stderr are bounded in memory and withheld from durable logs; execution summaries expose byte counts and status. Orphan execution after a daemon crash remains unknown and blocks replacement; the controller never signals a stored PID or relaunches it.

Repairs, finding resolution, full-pipeline automation, acceptance, cap decisions, recovery, repository executable authority, publication, forge operations, and the Posse adapter remain unsupported. An independent exact-head review of this implementation belongs to the subsequent Review Task; deterministic fake responses are not that review.
