package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

type fixOptions struct {
	Corpus   string
	Out      string
	Model    string
	Pi       string
	BaseURL  string
	Label    string
	Cases    []string
	Parallel int
	Settle   time.Duration
	Price    Price
}

// Price is USD per million tokens. It prices runs whose Pi events report no
// cost, which happens because the provider entry Greenhouse writes carries no
// prices.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

func (p Price) cost(u Usage) float64 {
	return (float64(u.Input)*p.Input + float64(u.Output)*p.Output + float64(u.CacheRead)*p.CacheRead + float64(u.CacheWrite)*p.CacheWrite) / 1e6
}

type FixConfig struct {
	Label             string         `json:"label"`
	Started           time.Time      `json:"started"`
	Finished          time.Time      `json:"finished"`
	Model             string         `json:"model"`
	Backend           string         `json:"backend"`
	PiVersion         string         `json:"pi_version"`
	BaseURL           string         `json:"base_url"`
	GreenhouseVersion string         `json:"greenhouse_version"`
	GoVersion         string         `json:"go_version"`
	CorpusVersion     int            `json:"corpus_version"`
	CorpusRevision    int            `json:"corpus_revision"`
	CorpusDigest      string         `json:"corpus_digest"`
	Cases             []string       `json:"cases"`
	Parallel          int            `json:"parallel"`
	Settings          map[string]any `json:"settings"`
	Checks            [][]string     `json:"checks"`
	Price             Price          `json:"price_usd_per_mtok"`
	BarFixedRate      float64        `json:"bar_fixed_rate"`
}

type FixRecord struct {
	ID               string            `json:"id"`
	Target           string            `json:"target"`
	Candidate        string            `json:"candidate"`
	ReviewStatus     string            `json:"review_status"`
	Selected         map[string]string `json:"selected"`
	Status           string            `json:"status"`
	Log              string            `json:"log"`
	Responses        json.RawMessage   `json:"responses"`
	RepairHead       string            `json:"repair_head"`
	Diff             string            `json:"diff"`
	Checks           json.RawMessage   `json:"checks"`
	Args             []string          `json:"args"`
	InputDigest      string            `json:"input_digest"`
	InputVerified    bool              `json:"input_verified"`
	ReportedProvider string            `json:"reported_provider"`
	ReportedModel    string            `json:"reported_model"`
	ThinkingLevel    string            `json:"thinking_level"`
	Usage            Usage             `json:"usage"`
	CostSource       string            `json:"cost_source"`
	LatencyMillis    int64             `json:"latency_ms"`
	Tests            TestOutcomes      `json:"tests"`
	TestOutput       string            `json:"test_output,omitempty"`
	Error            string            `json:"error,omitempty"`
	Score            FixScore          `json:"score"`
}

type FixReport struct {
	Config        FixConfig    `json:"config"`
	Summary       FixSummary   `json:"summary"`
	CostUSD       float64      `json:"cost_usd"`
	LatencyMillis LatencyStats `json:"latency_ms"`
	Cases         []FixRecord  `json:"cases"`
}

func treeAt(repo, commit string) (map[string]string, error) {
	names, err := command(repo, gitEnv, "git", "ls-tree", "-r", "--name-only", commit)
	if err != nil {
		return nil, err
	}
	tree := map[string]string{}
	for _, name := range strings.Split(names, "\n") {
		if name == "" {
			continue
		}
		c := exec.Command("git", "show", commit+":"+name)
		c.Dir = repo
		c.Env = append(os.Environ(), gitEnv...)
		b, err := c.Output()
		if err != nil {
			return nil, fmt.Errorf("git show %s:%s: %w", commit, name, err)
		}
		tree[name] = string(b)
	}
	return tree, nil
}

func (a axiClient) settle(timeout time.Duration, field string, busy ...string) (map[string]any, map[string]any, error) {
	deadline := time.Now().Add(timeout)
	for {
		v, err := a.call(map[string]any{"op": "inspect"})
		if err != nil {
			return nil, nil, err
		}
		run, _ := v["run"].(map[string]any)
		part, _ := run[field].(map[string]any)
		status, _ := run["status"].(string)
		if part != nil && !slices.Contains(busy, status) {
			return v, part, nil
		}
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("%s did not settle", field)
		}
		time.Sleep(time.Second)
	}
}

func runFixCase(o fixOptions, bin, shimBin string, checks [][]string, settings map[string]any, work string, k FixCase, capture string) FixRecord {
	rec := FixRecord{ID: k.ID, Selected: map[string]string{}, Tests: TestOutcomes{}}
	fail := func(err error) FixRecord {
		rec.Error = err.Error()
		if rec.Status == "" {
			rec.Status = "eval-error"
		}
		rec.Score = ScoreFix(k, nil, k.Head, false, nil)
		return rec
	}
	repo, home := filepath.Join(work, "repo"), filepath.Join(work, "state")
	for _, dir := range []string{home, capture} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fail(err)
		}
	}
	target, head, err := buildRepo(repo, Case{Base: k.Base, Head: k.Head, Intent: k.Intent})
	if err != nil {
		return fail(err)
	}
	rec.Target, rec.Candidate = target, head
	cfg, _ := json.Marshal(shimConfig{Pi: o.Pi, Capture: capture, Review: k.Findings})
	cfgPath := filepath.Join(work, "shim.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return fail(err)
	}
	files := map[string]any{
		"backends.json": map[string]any{"version": 2, "writable": map[string][]string{"fixer": {capture}}},
		"fixer.json":    map[string]any{"version": 2, "name": "Eval Fixer", "email": "fixer@example.invalid", "checks": checks},
	}
	for name, body := range files {
		b, _ := json.Marshal(body)
		if err := os.WriteFile(filepath.Join(home, name), b, 0o600); err != nil {
			return fail(err)
		}
	}
	native := filepath.Join(work, "bin")
	if err := os.MkdirAll(native, 0700); err != nil {
		return fail(err)
	}
	if err := os.Symlink(shimBin, filepath.Join(native, "pi")); err != nil {
		return fail(err)
	}
	a := axiClient{bin: bin, repo: repo, env: []string{"GREENHOUSE_HOME=" + home, shimEnv + "=" + cfgPath, "PATH=" + native + ":" + os.Getenv("PATH")}}
	defer stopDaemon(home)
	v, err := a.call(map[string]any{"op": "attach"})
	if err != nil {
		return fail(err)
	}
	applied := maps.Clone(settings)
	applied["rules"] = k.Rules
	if v, err = a.mutate(v, "apply", map[string]any{"settings": applied, "reason": "fixer evaluation case " + k.ID}); err != nil {
		return fail(err)
	}
	if v, err = a.mutate(v, "resolve-rules", nil); err != nil {
		return fail(err)
	}
	if v, err = a.mutate(v, "prepare", map[string]any{"intent": k.Intent, "target": target}); err != nil {
		return fail(err)
	}
	if _, err = a.mutate(v, "review", nil); err != nil {
		return fail(err)
	}
	v, review, err := a.settle(time.Minute, "review", "running", "cancelling")
	if err != nil {
		return fail(err)
	}
	rec.ReviewStatus, _ = review["status"].(string)
	var result struct {
		Findings []struct {
			ID string `json:"id"`
			Finding
		} `json:"findings"`
	}
	b, _ := json.Marshal(review["result"])
	json.Unmarshal(b, &result)
	labels := map[string]string{}
	var ids []string
	for _, f := range result.Findings {
		for _, s := range k.Findings {
			if s.File == f.File && s.Start == f.Start && s.End == f.End && s.Explanation == f.Explanation {
				labels[f.ID] = s.Label
				if s.Selected {
					rec.Selected[s.Label] = f.ID
					ids = append(ids, f.ID)
				}
			}
		}
	}
	if len(ids) != len(k.selectedLabels()) {
		return fail(fmt.Errorf("seeded review produced %d of %d selected findings (review %s: %v)", len(ids), len(k.selectedLabels()), rec.ReviewStatus, review["log"]))
	}
	if _, err = a.mutate(v, "repair", map[string]any{"selected": ids}); err != nil {
		return fail(err)
	}
	_, repair, err := a.settle(o.Settle, "repair", "writing", "cancelling")
	if err != nil {
		return fail(err)
	}
	var x struct {
		Status      string          `json:"status"`
		Log         string          `json:"log"`
		Head        string          `json:"head"`
		Diff        string          `json:"diff"`
		Args        []string        `json:"args"`
		Input       []byte          `json:"input"`
		InputDigest string          `json:"input_digest"`
		Checks      json.RawMessage `json:"checks"`
		Responses   []struct {
			Finding  string `json:"finding"`
			Response string `json:"response"`
		} `json:"responses"`
	}
	b, _ = json.Marshal(repair)
	if err := json.Unmarshal(b, &x); err != nil {
		return fail(err)
	}
	rec.Status, rec.Log, rec.RepairHead, rec.Diff, rec.Args, rec.InputDigest, rec.Checks = x.Status, x.Log, x.Head, x.Diff, x.Args, x.InputDigest, x.Checks
	rec.Responses, _ = json.Marshal(repair["responses"])
	delivered, err := os.ReadFile(filepath.Join(capture, "input.json"))
	rec.InputVerified = err == nil && bytes.Equal(delivered, x.Input)
	events, _ := os.ReadFile(filepath.Join(capture, "events.jsonl"))
	p := parseEvents(events)
	if err := os.WriteFile(filepath.Join(capture, "events.jsonl"), compactEvents(events), 0o600); err != nil {
		return fail(err)
	}
	rec.ReportedProvider, rec.ReportedModel, rec.ThinkingLevel, rec.Usage = p.Provider, p.Model, p.Thinking, p.Usage
	rec.CostSource = "pi"
	if rec.Usage.CostUSD == 0 {
		rec.Usage.CostUSD, rec.CostSource = o.Price.cost(rec.Usage), "tokens at the configured price"
	}
	var timing shimTiming
	if t, err := os.ReadFile(filepath.Join(capture, "timing.json")); err == nil && json.Unmarshal(t, &timing) == nil {
		rec.LatencyMillis = timing.Millis
	}
	repaired := k.Head
	if x.Head != "" {
		if repaired, err = treeAt(repo, x.Head); err != nil {
			return fail(err)
		}
	}
	responses := map[string]string{}
	for _, r := range x.Responses {
		responses[labels[r.Finding]] = r.Response
	}
	var testErr error
	rec.Tests, rec.TestOutput, testErr = runTests(filepath.Join(work, "tests"), repaired, k.Hidden)
	if testErr != nil {
		return fail(testErr)
	}
	rec.Score = ScoreFix(k, responses, repaired, x.Status == "scope-refused", rec.Tests)
	if rec.Score.Fixed {
		rec.TestOutput = ""
	}
	return rec
}

func runFixLive(o fixOptions) (*FixReport, error) {
	c, err := LoadFixCorpus(o.Corpus)
	if err != nil {
		return nil, err
	}
	cases := c.Cases
	if len(o.Cases) > 0 {
		byID := map[string]FixCase{}
		for _, k := range c.Cases {
			byID[k.ID] = k
		}
		cases = nil
		for _, id := range o.Cases {
			k, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown case %s", id)
			}
			cases = append(cases, k)
		}
	}
	if o.BaseURL != "" {
		return nil, errors.New("configure provider routing in the native Pi configuration; -base-url is retired")
	}
	if o.Out, err = filepath.Abs(o.Out); err != nil {
		return nil, err
	}
	if _, err := os.Stat(o.Out); err == nil {
		return nil, fmt.Errorf("output %s already exists", o.Out)
	}
	root, err := command("", nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "gf")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "greenhouse")
	if _, err := command(root, nil, "go", "build", "-o", bin, "./cmd/greenhouse"); err != nil {
		return nil, err
	}
	shimBin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}
	if goBin, err = filepath.EvalSymlinks(goBin); err != nil {
		return nil, err
	}
	piVersion, err := command("", nil, o.Pi, "--version")
	if err != nil {
		return nil, err
	}
	version, err := command(root, nil, "git", "describe", "--always", "--dirty", "--abbrev=40")
	if err != nil {
		return nil, err
	}
	checks := [][]string{{goBin, "vet", "./..."}, {goBin, "test", "-p", "1", "-parallel", "1", "./..."}}
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "reviewer_backend": "pi", "reviewer_model": o.Model, "fixer_backend": "pi", "fixer_model": o.Model}
	r := &FixReport{Config: FixConfig{Label: o.Label, Started: time.Now().UTC(), Model: o.Model, Backend: "pi-json-v1", PiVersion: piVersion, BaseURL: o.BaseURL, GreenhouseVersion: version, GoVersion: runtime.Version(), CorpusVersion: c.Version, CorpusRevision: c.Revision, CorpusDigest: c.Digest, Parallel: o.Parallel, Settings: settings, Checks: checks, Price: o.Price, BarFixedRate: barFixedRate}}
	records := make([]FixRecord, len(cases))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.Parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				k := cases[i]
				records[i] = runFixCase(o, bin, shimBin, checks, settings, filepath.Join(work, fmt.Sprintf("f%02d", i)), k, filepath.Join(o.Out, "cases", k.ID))
				s := records[i].Score
				fmt.Fprintf(os.Stderr, "%s: %s response=%s accept=%d/%d regressions=%d scope=%d fixed=%t %dms\n", k.ID, records[i].Status, s.Response, s.AcceptPassed, s.AcceptTotal, len(s.Regressions), len(s.ScopeViolations), s.Fixed, records[i].LatencyMillis)
			}
		}()
	}
	for i := range cases {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	r.Config.Finished = time.Now().UTC()
	var scores []FixScore
	for _, rec := range records {
		r.Config.Cases = append(r.Config.Cases, rec.ID)
		r.CostUSD += rec.Usage.CostUSD
		r.LatencyMillis.Total += rec.LatencyMillis
		r.LatencyMillis.Max = max(r.LatencyMillis.Max, rec.LatencyMillis)
		scores = append(scores, rec.Score)
	}
	if len(records) > 0 {
		r.LatencyMillis.Mean = r.LatencyMillis.Total / int64(len(records))
	}
	r.Cases = records
	r.Summary = SummarizeFix(scores)
	if err := writeFixReport(o.Out, r); err != nil {
		return nil, err
	}
	return r, redactTree(o.Out)
}

func writeFixReport(dir string, r *FixReport) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	s := r.Summary
	var md strings.Builder
	fmt.Fprintf(&md, "# Fixer evaluation: %s\n\n", r.Config.Label)
	fmt.Fprintf(&md, "- Fixer: `%s` through `%s`, Pi %s; the review is seeded from each case\n", r.Config.Model, r.Config.Backend, r.Config.PiVersion)
	fmt.Fprintf(&md, "- Greenhouse: `%s`, fixer corpus v%d revision %d `%s`\n", r.Config.GreenhouseVersion, r.Config.CorpusVersion, r.Config.CorpusRevision, r.Config.CorpusDigest)
	fmt.Fprintf(&md, "- Started %s, finished %s, parallel %d\n\n", r.Config.Started.Format(time.RFC3339), r.Config.Finished.Format(time.RFC3339), r.Config.Parallel)
	bar := "not met"
	if s.BarMet {
		bar = "met"
	}
	fmt.Fprintf(&md, "Bar (at least %.0f%% of cases fixed, no regression, no scope violation): **%s**\n\n", r.Config.BarFixedRate*100, bar)
	fmt.Fprintf(&md, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&md, "| Fixed | %s (%d/%d) |\n", pct(s.FixedRate), s.Fixed, s.Cases)
	fmt.Fprintf(&md, "| Hidden acceptance tests passing | %d/%d |\n", s.AcceptPassed, s.AcceptTotal)
	fmt.Fprintf(&md, "| Regressions (cases / failing tests) | %d / %d |\n", s.RegressionCases, s.Regressions)
	fmt.Fprintf(&md, "| Scope violations (cases / edits) | %d / %d |\n", s.ScopeCases, s.ScopeViolations)
	fmt.Fprintf(&md, "| Correct resolution responses | %d/%d |\n", s.ResponsesCorrect, s.Cases)
	fmt.Fprintf(&md, "| Cost | $%.4f |\n", r.CostUSD)
	fmt.Fprintf(&md, "| Fixer latency total / mean / max | %.1fs / %.1fs / %.1fs |\n\n", float64(r.LatencyMillis.Total)/1000, float64(r.LatencyMillis.Mean)/1000, float64(r.LatencyMillis.Max)/1000)
	fmt.Fprintf(&md, "| Case | Repair status | Response | Acceptance | Regressions | Scope violations | Fixed | Cost | Latency |\n|---|---|---|---|---|---|---|---|---|\n")
	recs := append([]FixRecord(nil), r.Cases...)
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	for _, c := range recs {
		fmt.Fprintf(&md, "| %s | %s | %s | %d/%d | %s | %s | %t | $%.4f | %.1fs |\n", c.ID, c.Status, c.Score.Response, c.Score.AcceptPassed, c.Score.AcceptTotal, listOrNone(c.Score.Regressions), listOrNone(c.Score.ScopeViolations), c.Score.Fixed, c.Usage.CostUSD, float64(c.LatencyMillis)/1000)
	}
	fmt.Fprintf(&md, "\nPer-case `input.json` holds the exact bytes Greenhouse delivered to the fixer on stdin, `argv.json` the exact arguments, and `events.jsonl` the fixer's JSON event stream. `report.json` holds each repair diff, check result, resolution response and hidden test outcome.\n")
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(md.String()), 0o644)
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, "; ")
}
