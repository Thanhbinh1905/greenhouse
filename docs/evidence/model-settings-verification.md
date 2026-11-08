# Model-settings verification

## Merge-base red-before

I copied `settings_models_test.go` and `provider_config_test.go` to a temporary worktree at merge-base `c3572775536a1c962f96501a4ea617a979959076`, then ran their tests there. The base does not contain the reviewer/fixer model or provider implementation, so both copied test files fail compilation before any test body runs.

- Settings command: `go test ./... -run '^(TestApplyAndSaveReviewerAndFixerModels|TestReviewerModelChangeStalesHistoricalVerdict|TestApplyRejectsInvalidBackendAndModelIdentifiers)$' -count=1`
- Settings failure: `saved.ReviewerModel undefined (type Settings has no field or method ReviewerModel)` and `unknown field ReviewerBackend in struct literal of type Settings`.
- Provider command: `go test ./... -run '^TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials$' -count=1`
- Provider failure: `undefined: TrustedProviderConfig`, `undefined: trustedProviders`, `undefined: providerModelCatalog`, `undefined: preflightModel`, `undefined: writeAgentModels`, and `undefined: PiModelsFile`.

## Provider behavioral red-before

Before the provider fixes, the focused provider regression also ran and failed at runtime:

- Command: `go test ./... -run '^TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials$' -count=1`
- Failure: `provider_config_test.go:51: preflight accepted a locally declared model absent from the provider catalog`.
- A separate isolated-Pi-config check failed with `provider_config_test.go:69: selected model not materialized in isolated Pi config despite being reported by Pi`.

## Green-after

- Command: `go test ./... -run '^(TestApplyAndSaveReviewerAndFixerModels|TestReviewerModelChangeStalesHistoricalVerdict|TestApplyRejectsInvalidBackendAndModelIdentifiers|TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials)$' -count=1`
- Result: `ok greenhouse`; `ok greenhouse/eval [no tests to run]`.
- The Settings test covers separate reviewer and fixer selections, Apply without changing saved defaults, reattach between Apply and Save, and Save persistence. The other Settings tests cover invalid identifiers and stale historical review evidence.
- The provider test covers trusted model discovery, environment-based credentials, and provider preflight.

## Product path

The product smoke used the built reviewer through AXI against the configured local proxy, not the eval harness. The smoke returned `clean-review` for requested model `openai/gpt-5.6-luna`; the reported provider was `openai`, and the reported model was `gpt-5.6-luna`. Coverage included `review.go`. The credential was injected into the smoke process environment and did not appear in output or evidence.
