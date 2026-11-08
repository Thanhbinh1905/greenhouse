package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderCatalogUsesPrivateConfigAndEnvironmentCredentials(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.Chmod(homeDir, 0700); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-secret" {
			http.Error(w, "bad request", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "gpt-5.6-luna"}}})
	}))
	defer endpoint.Close()
	config := TrustedProviderConfig{Providers: map[string]ProviderConfig{"openai": {BaseURL: endpoint.URL + "/v1", APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY"}}, Models: []string{"openai/gpt-5.6-luna"}}
	data, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(homeDir, "providers.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREENHOUSE_HOME", homeDir)
	t.Setenv("GREENHOUSE_TEST_PROVIDER_KEY", "test-secret")
	loaded, err := trustedProviders()
	if err != nil || loaded.Providers["openai"].API != "openai-completions" {
		t.Fatalf("OpenAI-compatible API default missing: %+v, %v", loaded, err)
	}
	provider := loaded.Providers["openai"]
	pi := filepath.Join(homeDir, "pi")
	script := "#!/bin/sh\nprintf 'provider model\\nopenai gpt-5.6-luna\\n'\n"
	if err := os.WriteFile(pi, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	catalog, err := providerModelCatalog(pi)
	if err != nil || !catalog["openai/gpt-5.6-luna"] {
		t.Fatalf("Pi/provider model missing: %v, %v", catalog, err)
	}
	agent := filepath.Join(homeDir, "agent")
	if err := writeAgentModels(agent, ReviewerConfig{Executable: "/bin/true", Model: "openai/gpt-5.6-luna", Provider: provider}, pi); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(agent, "models.json"))
	var piConfig PiModelsFile
	if err != nil || json.Unmarshal(content, &piConfig) != nil || piConfig.Providers["openai"].APIKey != "$GREENHOUSE_TEST_PROVIDER_KEY" {
		t.Fatalf("isolated provider config did not retain only the key environment-variable name: %s, %v", content, err)
	}
	if err := preflightModel("openai/gpt-5.6-luna"); err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	if err := preflightModel("openai/nonexistent"); err == nil {
		t.Fatal("preflight accepted a model absent from the provider catalog")
	}
	invalidAPI := TrustedProviderConfig{Providers: map[string]ProviderConfig{"openai": {BaseURL: endpoint.URL, APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY", API: "anthropic-messages"}}}
	invalid, _ := json.Marshal(invalidAPI)
	if err := os.WriteFile(filepath.Join(homeDir, "providers.json"), invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := trustedProviders(); err == nil {
		t.Fatal("provider accepted an API whose preflight contract is unsupported")
	}
	if err := os.WriteFile(filepath.Join(homeDir, "providers.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREENHOUSE_TEST_PROVIDER_KEY", "")
	if err := preflightModel("openai/gpt-5.6-luna"); err == nil {
		t.Fatal("preflight accepted missing credentials")
	}
	contents, err := os.ReadFile(filepath.Join(homeDir, "providers.json"))
	if err != nil || string(contents) == "" || string(contents) == "test-secret" {
		t.Fatal("credential was stored in provider configuration")
	}
}
