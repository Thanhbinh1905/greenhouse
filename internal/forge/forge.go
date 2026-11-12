package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	Head   string `json:"head"`
}

type Forge interface {
	EnsurePullRequest(context.Context, string, string, string, string, string, string, string) (PullRequest, error)
	Checks(context.Context, string, string, string, int, []string) (bool, string, error)
}

type GitHubForge struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (g GitHubForge) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (g GitHubForge) request(ctx context.Context, method, path string, body any, out any) error {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("forge returned HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
	}
	return nil
}

func (g GitHubForge) EnsurePullRequest(ctx context.Context, owner, repo, branch, base, head, title, body string) (PullRequest, error) {
	var found []struct {
		Number int    `json:"number"`
		URL    string `json:"html_url"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Base   struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls?state=open&per_page=100&head=" + url.QueryEscape(owner+":"+branch)
	if err := g.request(ctx, http.MethodGet, path, nil, &found); err != nil {
		return PullRequest{}, err
	}
	if len(found) > 1 {
		return PullRequest{}, errors.New("multiple open pull requests match the configured branch")
	}
	if len(found) == 1 {
		item := found[0]
		if item.Number == 0 || item.URL == "" {
			return PullRequest{}, errors.New("existing pull request is missing identity")
		}
		if item.Head.SHA == head && item.Title == title && item.Body == body && item.Base.Ref == base {
			return PullRequest{Number: item.Number, URL: item.URL, Head: item.Head.SHA}, nil
		}
		var updated struct {
			Number int    `json:"number"`
			URL    string `json:"html_url"`
			Head   struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		if err := g.request(ctx, http.MethodPatch, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls/"+fmt.Sprint(item.Number), map[string]string{"title": title, "body": body, "base": base}, &updated); err != nil {
			return PullRequest{}, err
		}
		if updated.Head.SHA != head {
			return PullRequest{}, errors.New("pull request does not point to the requested head")
		}
		return PullRequest{Number: updated.Number, URL: updated.URL, Head: updated.Head.SHA}, nil
	}
	var created struct {
		Number int    `json:"number"`
		URL    string `json:"html_url"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := g.request(ctx, http.MethodPost, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls", map[string]string{"title": title, "body": body, "head": branch, "base": base}, &created); err != nil {
		return PullRequest{}, err
	}
	if created.Number == 0 || created.URL == "" {
		return PullRequest{}, errors.New("forge created a PR without identity")
	}
	if created.Head.SHA != head {
		return PullRequest{}, errors.New("created pull request does not point to the requested head")
	}
	return PullRequest{Number: created.Number, URL: created.URL, Head: created.Head.SHA}, nil
}

func (g GitHubForge) Checks(ctx context.Context, owner, repo, head string, pullNumber int, required []string) (bool, string, error) {
	var result struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Name       string  `json:"name"`
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion"`
			HeadSHA    string  `json:"head_sha"`
		} `json:"check_runs"`
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/commits/" + url.PathEscape(head) + "/check-runs?per_page=100"
	if err := g.request(ctx, http.MethodGet, path, nil, &result); err != nil {
		return false, "", err
	}
	if pullNumber <= 0 || len(required) == 0 {
		return false, "trusted PR identity and required check names are missing", nil
	}
	var pr struct {
		Number int `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := g.request(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/pulls/"+fmt.Sprint(pullNumber), nil, &pr); err != nil {
		return false, "", err
	}
	if pr.Number != pullNumber || pr.Head.SHA != head {
		return false, "pull request head moved from authorized candidate", nil
	}
	if len(result.CheckRuns) == 0 {
		return false, "missing required check: " + required[0], nil
	}
	if result.TotalCount > len(result.CheckRuns) {
		return false, "checks incomplete: additional pages remain", nil
	}
	byName := make(map[string][]struct {
		Status     string
		Conclusion *string
		HeadSHA    string
	}, len(result.CheckRuns))
	for _, check := range result.CheckRuns {
		byName[check.Name] = append(byName[check.Name], struct {
			Status     string
			Conclusion *string
			HeadSHA    string
		}{check.Status, check.Conclusion, check.HeadSHA})
	}
	for _, name := range required {
		checks := byName[name]
		if len(checks) == 0 {
			return false, "missing required check: " + name, nil
		}
		if len(checks) != 1 {
			return false, "duplicate required check: " + name, nil
		}
		check := checks[0]
		if check.HeadSHA != head {
			return false, "stale check: " + name, nil
		}
		if check.Status != "completed" || check.Conclusion == nil {
			return false, "checks pending: " + name, nil
		}
		if *check.Conclusion != "success" {
			return false, "check failed: " + name + " (" + *check.Conclusion + ")", nil
		}
	}
	return true, "all required checks passed for exact head", nil
}

func Configured() (GitHubForge, error) {
	base := os.Getenv("GREENHOUSE_GITHUB_API")
	if base != "" {
		parsed, err := url.Parse(base)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return GitHubForge{}, errors.New("invalid GREENHOUSE_GITHUB_API")
		}
	}
	token := ""
	if base == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return GitHubForge{BaseURL: base, Token: token}, nil
}
