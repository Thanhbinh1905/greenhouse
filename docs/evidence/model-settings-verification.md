# Model-settings verification

## Model selection red-before

`TestApplyAndSaveReviewerAndFixerModels` was introduced with the feature, so it could not run against the pre-feature commit. A temporary AXI regression test ran against a scratch clone at `f8420b9`:

- Command: `cd /home/thanhbinh/.posse/scratch/greenhouse/t39/red-baseline && go test -run '^TestRedBeforeReviewerAndFixerModelSelection$' -count=1`
- Failure: `json: unknown field "fixer_model"`. The pre-feature AXI rejected the reviewer/fixer model request before Apply could store the selections.

## Provider red-before

The provider regression test failed before the provider fixes:

- Command: `go test ./... -run '^TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials$' -count=1`
- Failure: `provider_config_test.go:51: preflight accepted a locally declared model absent from the provider catalog`.
- A separate isolated-Pi-config check failed with `provider_config_test.go:69: selected model not materialized in isolated Pi config despite being reported by Pi`.

## Green-after

- Command: `go test -p 1 ./... -run '^(TestApplyAndSaveReviewerAndFixerModels|TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials)$' -count=1`
- Result: `ok greenhouse`; `ok greenhouse/eval [no tests to run]`.
- The Settings test covers separate reviewer and fixer selections, Apply without changing saved defaults, and Save persistence. The provider test covers trusted model discovery, environment-based credentials, and provider preflight.

## Product path

The product smoke used the built reviewer through AXI against the configured local proxy, not the eval harness. The smoke returned `clean-review` for requested model `openai/gpt-5.6-luna`; the reported provider was `openai`, and the reported model was `gpt-5.6-luna`. Coverage included `review.go`. The credential was injected into the smoke process environment and did not appear in output or evidence.
