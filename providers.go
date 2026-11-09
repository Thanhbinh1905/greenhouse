package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ProviderConfig struct {
	BaseURL   string `json:"base_url"`
	APIKeyEnv string `json:"api_key_env"`
	API       string `json:"api"`
}
type TrustedProviderConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
	Models    []string                  `json:"models,omitempty"`
}
type PiModelConfig struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	API           string   `json:"api,omitempty"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	ContextWindow int      `json:"contextWindow"`
	MaxTokens     int      `json:"maxTokens"`
}
type PiModelsFile struct {
	Providers map[string]PiProviderEntry `json:"providers"`
}
type PiProviderEntry struct {
	BaseURL string          `json:"baseUrl"`
	APIKey  string          `json:"apiKey"`
	API     string          `json:"api,omitempty"`
	Models  []PiModelConfig `json:"models,omitempty"`
}

func trustedProviders() (TrustedProviderConfig, error) {
	var config TrustedProviderConfig
	h, err := home()
	if err != nil {
		return config, err
	}
	path := filepath.Join(h, "providers.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return config, errors.New("private per-user providers.json required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err = decodeConfig("providers.json", data, &config); err != nil {
		return config, configFailure(err, "invalid per-user providers.json")
	}
	for provider, entry := range config.Providers {
		parsed, parseErr := url.Parse(entry.BaseURL)
		if !validModelSelection("pi", provider+"/id") || parseErr != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !validEnvName(entry.APIKeyEnv) || (entry.API != "" && entry.API != "openai-completions" && entry.API != "openai-responses") {
			return config, errors.New("provider requires an OpenAI-compatible API, base_url, and API key environment-variable name")
		}
		if entry.API == "" {
			entry.API = "openai-completions"
			config.Providers[provider] = entry
		}
	}
	for _, model := range config.Models {
		if !validModelSelection("pi", model) || config.Providers[strings.SplitN(model, "/", 2)[0]].BaseURL == "" {
			return config, fmt.Errorf("invalid or unconfigured provider model %q", model)
		}
	}
	return config, nil
}

func validEnvName(name string) bool {
	if name == "" || strings.HasPrefix(name, "PI_") || strings.HasPrefix(name, "POSSE_") || strings.HasPrefix(name, "HERDR_") || strings.HasPrefix(name, "GIT_") || !((name[0] >= 'A' && name[0] <= 'Z') || name[0] == '_') {
		return false
	}
	for _, c := range name[1:] {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func providerModelCatalog(executable string) (map[string]bool, error) {
	command := exec.Command(executable, "--list-models")
	command.Env = reviewerEnv()
	output, err := command.Output()
	catalog := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !(strings.ToLower(fields[0]) == "provider" && strings.ToLower(fields[1]) == "model") && validModelSelection("pi", fields[0]+"/"+fields[1]) {
			catalog[fields[0]+"/"+fields[1]] = true
		}
	}
	providers, providerErr := trustedProviders()
	if providerErr != nil {
		return nil, providerErr
	}
	for _, model := range providers.Models {
		catalog[model] = true
	}
	if err != nil && len(catalog) == 0 {
		return nil, errors.New("trusted Pi model catalog is unavailable")
	}
	return catalog, nil
}

// Pi forces thinking off when a custom provider entry omits reasoning.
func catalogReasoning(executable, model string) bool {
	command := exec.Command(executable, "--list-models")
	command.Env = reviewerEnv()
	output, _ := command.Output()
	column := -1
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "provider") && strings.EqualFold(fields[1], "model") {
			column = slices.Index(fields, "thinking")
			continue
		}
		if column >= 0 && len(fields) > column && fields[0]+"/"+fields[1] == model {
			return fields[column] == "yes"
		}
	}
	return false
}

func preflightModel(model string) error {
	provider, modelID, found := strings.Cut(model, "/")
	if !found {
		return errors.New("model must be provider-qualified")
	}
	config, err := trustedProviders()
	if err != nil {
		return err
	}
	entry, ok := config.Providers[provider]
	if !ok {
		return fmt.Errorf("provider %s is unconfigured; add its endpoint and key environment-variable name to private providers.json", provider)
	}
	key, ok := os.LookupEnv(entry.APIKeyEnv)
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("provider %s is unconfigured: environment variable %s is empty", provider, entry.APIKeyEnv)
	}
	client := &http.Client{Timeout: 4 * time.Second}
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(entry.BaseURL, "/")+"/models", nil)
	if err != nil {
		return fmt.Errorf("provider %s is unreachable", provider)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("provider %s is unreachable", provider)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider %s is unreachable or rejected credentials (HTTP %d)", provider, response.StatusCode)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return fmt.Errorf("provider %s returned an invalid model catalog", provider)
	}
	for _, candidate := range result.Data {
		if candidate.ID == modelID {
			return nil
		}
	}
	return fmt.Errorf("model %s is not reported by provider %s", model, provider)
}

func trustedReviewerFor(model string) (ReviewerConfig, error) {
	config, err := trustedReviewer()
	if err != nil {
		return config, err
	}
	provider, _, found := strings.Cut(model, "/")
	if !found {
		return config, errors.New("model must be provider-qualified")
	}
	providers, err := trustedProviders()
	if err != nil {
		return config, err
	}
	entry, ok := providers.Providers[provider]
	if !ok {
		return config, fmt.Errorf("provider %s is unconfigured; add its endpoint and key environment-variable name to private providers.json", provider)
	}
	config.Model = model
	config.Provider = entry
	return config, nil
}

func writeAgentModels(agentDir string, config ReviewerConfig) error {
	provider, modelID, ok := strings.Cut(config.Model, "/")
	if !ok || config.Provider.BaseURL == "" {
		return errors.New("model provider configuration is unavailable")
	}
	api := config.Provider.API
	if api == "" {
		return errors.New("custom model provider requires an api in providers.json")
	}
	piEntry := PiProviderEntry{
		BaseURL: config.Provider.BaseURL,
		APIKey:  "$" + config.Provider.APIKeyEnv,
		API:     api,
		Models:  []PiModelConfig{{ID: modelID, Name: modelID, API: api, Reasoning: catalogReasoning(config.Executable, config.Model), Input: []string{"text"}, ContextWindow: 128000, MaxTokens: 16384}},
	}
	if err := os.MkdirAll(agentDir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(PiModelsFile{Providers: map[string]PiProviderEntry{provider: piEntry}})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(agentDir, "models.json"), data, 0600)
}

func preflightPi(config ReviewerConfig) error {
	if config.Backend != "" && config.Backend != backendPi {
		return nil
	}
	catalog, err := providerModelCatalog(config.Executable)
	if err != nil {
		return err
	}
	if !catalog[config.Model] {
		return errors.New("model is not reported by Pi; add custom models to private providers.json")
	}
	return preflightModel(config.Model)
}
