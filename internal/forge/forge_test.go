package forge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubForgeReusesExistingPullRequest(t *testing.T) {
	creates, updates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/garden/pulls/7" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"number":7,"head":{"sha":"head-a"}}`))
			return
		}
		if r.URL.Path == "/repos/acme/garden/commits/head-a/check-runs" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"head-a"},{"name":"optional-lint","status":"completed","conclusion":"success","head_sha":"head-a"}]}`))
			return
		}
		if r.URL.Path == "/repos/acme/garden/pulls/7" && r.Method == http.MethodPatch {
			updates++
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["title"] != "title" || body["base"] != "main" {
				t.Errorf("unexpected PR update: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"number":7,"html_url":"https://example.invalid/acme/garden/pull/7","head":{"sha":"head-a"}}`))
			return
		}
		if r.URL.Path != "/repos/acme/garden/pulls" {
			t.Fatalf("unexpected forge path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"number":7,"html_url":"https://example.invalid/acme/garden/pull/7","head":{"sha":"head-a"}}]`))
		case http.MethodPost:
			creates++
			t.Fatal("existing PR should not be recreated")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	forge := GitHubForge{BaseURL: server.URL, Client: server.Client()}
	pr, err := forge.EnsurePullRequest(context.Background(), "acme", "garden", "topic", "main", "head-a", "title", "body")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 7 || pr.URL != "https://example.invalid/acme/garden/pull/7" || creates != 0 || updates != 1 {
		t.Fatalf("PR was not reconciled: %+v creates=%d", pr, creates)
	}
	passed, reason, err := forge.Checks(context.Background(), "acme", "garden", "head-a", 7, []string{"trusted-ci"})
	if err != nil || !passed || reason != "all required checks passed for exact head" {
		t.Fatalf("exact-head checks not accepted: passed=%t reason=%q err=%v", passed, reason, err)
	}
}

func TestGitHubForgeReconcilesAlreadyExactPullRequestWithoutMutation(t *testing.T) {
	var mutations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/garden/pulls" {
			json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "title": "title", "body": "body", "base": map[string]string{"ref": "main"}, "head": map[string]string{"sha": "head-a"}}})
			return
		}
		if r.Method == http.MethodPatch || r.Method == http.MethodPost {
			mutations++
		}
		http.Error(w, "unexpected request", http.StatusNotFound)
	}))
	defer server.Close()
	pr, err := (GitHubForge{BaseURL: server.URL, Client: server.Client()}).EnsurePullRequest(context.Background(), "acme", "garden", "topic", "main", "head-a", "title", "body")
	if err != nil || pr.Number != 12 || pr.Head != "head-a" || mutations != 0 {
		t.Fatalf("exact PR was mutated or not reconciled: pr=%+v mutations=%d err=%v", pr, mutations, err)
	}
}

func TestGitHubForgeMissingAndStaleChecksDoNotPass(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"missing", `{"check_runs":[]}`, "missing required check: trusted-ci"},
		{"stale", `{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"old-head"}]}`, "stale check: trusted-ci"},
		{"unknown", `{"check_runs":[{"name":"trusted-ci","status":"completed","head_sha":"head-a"}]}`, "checks pending: trusted-ci"},
		{"ancillary-only", `{"check_runs":[{"name":"optional-ci","status":"completed","conclusion":"success","head_sha":"head-a"}]}`, "missing required check: trusted-ci"},
		{"pr-head-moved", `{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"head-a"}]}`, "pull request head moved from authorized candidate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/repos/acme/garden/pulls/4" {
					sha := "head-a"
					if tc.name == "pr-head-moved" {
						sha = "head-b"
					}
					w.Write([]byte(`{"number":4,"head":{"sha":"` + sha + `"}}`))
					return
				}
				w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			passed, reason, err := (GitHubForge{BaseURL: server.URL, Client: server.Client()}).Checks(context.Background(), "acme", "garden", "head-a", 4, []string{"trusted-ci"})
			if err != nil || passed || reason != tc.want {
				t.Fatalf("unexpected result: %t %q %v", passed, reason, err)
			}
		})
	}
}
