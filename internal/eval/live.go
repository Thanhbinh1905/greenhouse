package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type runOptions struct {
	Split      string
	JudgeModel string
	Corpus     string
	Out        string
	Model      string
	Pi         string
	Backend    string
	Agent      string
	BaseURL    string
	Label      string
	Cases      []string
	Parallel   int
	Settle     time.Duration
}

type RunConfig struct {
	Split             string    `json:"split"`
	Label             string    `json:"label"`
	Started           time.Time `json:"started"`
	Finished          time.Time `json:"finished"`
	Model             string    `json:"model"`
	Backend           string    `json:"backend"`
	PiVersion         string    `json:"pi_version"`
	BaseURL           string    `json:"base_url,omitempty"`
	GreenhouseVersion string    `json:"greenhouse_version"`
	GoVersion         string    `json:"go_version"`
	CorpusVersion     int       `json:"corpus_version"`
	CorpusRevision    int       `json:"corpus_revision"`
	CorpusDigest      string    `json:"corpus_digest"`
	ScoredRevision    int       `json:"scored_corpus_revision"`
	ScoredDigest      string    `json:"scored_corpus_digest"`
	Cases             []string  `json:"cases"`
	Parallel          int       `json:"parallel"`
	MaxSpan           int       `json:"max_span"`
	BarHighRecall     float64   `json:"bar_high_recall"`
}

type Usage struct {
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cache_read"`
	CacheWrite int     `json:"cache_write"`
	Reasoning  int     `json:"reasoning"`
	CostUSD    float64 `json:"cost_usd"`
}

type CaseRecord struct {
	ID               string          `json:"id"`
	Target           string          `json:"target"`
	Candidate        string          `json:"candidate"`
	Status           string          `json:"status"`
	Log              string          `json:"log"`
	Args             []string        `json:"args"`
	InputDigest      string          `json:"input_digest"`
	InputVerified    bool            `json:"input_verified"`
	ReportedProvider string          `json:"reported_provider"`
	ReportedModel    string          `json:"reported_model"`
	ThinkingLevel    string          `json:"thinking_level"`
	Usage            Usage           `json:"usage"`
	LatencyMillis    int64           `json:"latency_ms"`
	ModelAnswer      string          `json:"model_answer"`
	ModelFindings    json.RawMessage `json:"model_findings"`
	Findings         []Finding       `json:"findings"`
	Error            string          `json:"error,omitempty"`
	Score            CaseScore       `json:"score"`
}

type Report struct {
	Config        RunConfig    `json:"config"`
	Summary       Summary      `json:"summary"`
	CostUSD       float64      `json:"cost_usd"`
	LatencyMillis LatencyStats `json:"latency_ms"`
	Cases         []CaseRecord `json:"cases"`
}

type LatencyStats struct {
	Total int64 `json:"total"`
	Mean  int64 `json:"mean"`
	Max   int64 `json:"max"`
}

func command(dir string, env []string, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

var gitEnv = []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Eval", "GIT_AUTHOR_EMAIL=eval@example.invalid", "GIT_COMMITTER_NAME=Eval", "GIT_COMMITTER_EMAIL=eval@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}

func writeTree(root string, tree map[string]string) error {
	for name, body := range tree {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// buildRepo commits the base tree, then the head tree, with fixed identities
// and dates so commit IDs are stable across runs.
func buildRepo(dir string, k Case) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	if _, err := command(dir, gitEnv, "git", "init", "-q", "-b", "main"); err != nil {
		return "", "", err
	}
	if err := writeTree(dir, k.Base); err != nil {
		return "", "", err
	}
	if _, err := command(dir, gitEnv, "git", "add", "-A"); err != nil {
		return "", "", err
	}
	if _, err := command(dir, gitEnv, "git", "commit", "-q", "--allow-empty", "-m", "base"); err != nil {
		return "", "", err
	}
	target, err := command(dir, gitEnv, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	for name := range k.Base {
		if _, ok := k.Head[name]; !ok {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
				return "", "", err
			}
		}
	}
	if err := writeTree(dir, k.Head); err != nil {
		return "", "", err
	}
	if _, err := command(dir, gitEnv, "git", "add", "-A"); err != nil {
		return "", "", err
	}
	if _, err := command(dir, gitEnv, "git", "commit", "-q", "-m", k.Intent); err != nil {
		return "", "", err
	}
	head, err := command(dir, gitEnv, "git", "rev-parse", "HEAD")
	return target, head, err
}

type axiClient struct {
	bin, repo string
	env       []string
}

func (a axiClient) call(q map[string]any) (map[string]any, error) {
	q["version"] = 1
	b, _ := json.Marshal(q)
	c := exec.Command(a.bin, "axi")
	c.Dir = a.repo
	c.Env = append(os.Environ(), a.env...)
	c.Stdin = bytes.NewReader(b)
	out, err := c.Output()
	var v map[string]any
	if json.Unmarshal(out, &v) != nil {
		return nil, fmt.Errorf("axi %s: %v: %s", q["op"], err, out)
	}
	return v, nil
}

func (a axiClient) mutate(v map[string]any, op string, extra map[string]any) (map[string]any, error) {
	q := map[string]any{"op": op, "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "eval-" + op}
	maps.Copy(q, extra)
	r, err := a.call(q)
	if err != nil {
		return nil, err
	}
	if r["outcome"] != "ok" {
		return nil, fmt.Errorf("%s refused: %v", op, r["error"])
	}
	return r, nil
}

func stopDaemon(home string) {
	b, err := os.ReadFile(filepath.Join(home, "pid"))
	if err != nil {
		return
	}
	var pid int
	if json.Unmarshal(b, &pid) == nil && pid > 0 {
		syscall.Kill(pid, syscall.SIGTERM)
	}
}

type piEvents struct {
	Answer, Provider, Model, Thinking string
	Usage                             Usage
}

func parseEvents(b []byte) piEvents {
	var p piEvents
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 8*1024*1024)
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Role     string `json:"role"`
				Provider string `json:"provider"`
				Model    string `json:"model"`
				Thinking string `json:"thinkingLevel"`
				Content  []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				Usage struct {
					Input      int `json:"input"`
					Output     int `json:"output"`
					CacheRead  int `json:"cacheRead"`
					CacheWrite int `json:"cacheWrite"`
					Reasoning  int `json:"reasoning"`
					Cost       struct {
						Total float64 `json:"total"`
					} `json:"cost"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != "message_end" || ev.Message.Role != "assistant" {
			continue
		}
		m := ev.Message
		p.Provider, p.Model, p.Thinking = m.Provider, m.Model, m.Thinking
		p.Usage.Input += m.Usage.Input
		p.Usage.Output += m.Usage.Output
		p.Usage.CacheRead += m.Usage.CacheRead
		p.Usage.CacheWrite += m.Usage.CacheWrite
		p.Usage.Reasoning += m.Usage.Reasoning
		p.Usage.CostUSD += m.Usage.Cost.Total
		p.Answer = ""
		for _, c := range m.Content {
			if c.Type == "text" {
				p.Answer += c.Text
			}
		}
	}
	return p
}

var credentialExpiry = regexp.MustCompile(`"credential_expires_at":"[^"]*"`)

func parseClaudeEvents(b []byte) piEvents {
	p := piEvents{Provider: "claude"}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 8*1024*1024)
	for sc.Scan() {
		var ev struct {
			Type       string          `json:"type"`
			Result     string          `json:"result"`
			Structured json.RawMessage `json:"structured_output"`
			Cost       float64         `json:"total_cost_usd"`
			Usage      map[string]struct {
				Input      int `json:"inputTokens"`
				Output     int `json:"outputTokens"`
				CacheRead  int `json:"cacheReadInputTokens"`
				CacheWrite int `json:"cacheCreationInputTokens"`
				Thinking   int `json:"thinkingTokens"`
			} `json:"modelUsage"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type != "result" {
			continue
		}
		p.Answer = ev.Result
		if len(ev.Structured) > 0 && string(ev.Structured) != "null" {
			p.Answer = string(ev.Structured)
		}
		p.Usage.CostUSD = ev.Cost
		for model, u := range ev.Usage {
			p.Model = model
			p.Usage.Input, p.Usage.Output, p.Usage.CacheRead, p.Usage.CacheWrite, p.Usage.Reasoning = u.Input, u.Output, u.CacheRead, u.CacheWrite, u.Thinking
		}
	}
	return p
}

// compactEvents drops message_update events. They stream deltas whose final
// content the matching message_end event repeats.
func compactEvents(b []byte) []byte {
	var out bytes.Buffer
	for _, line := range bytes.SplitAfter(b, []byte("\n")) {
		var ev struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &ev) == nil && ev.Type == "message_update" {
			continue
		}
		out.Write(line)
	}
	return out.Bytes()
}

func modelFindings(answer string) json.RawMessage {
	var v struct {
		Findings json.RawMessage `json:"findings"`
	}
	if json.Unmarshal([]byte(answer), &v) != nil || v.Findings == nil {
		return json.RawMessage("null")
	}
	return v.Findings
}

func runCase(o runOptions, bin, shimBin, work string, k Case, capture string) CaseRecord {
	rec := CaseRecord{ID: k.ID}
	fail := func(err error) CaseRecord {
		rec.Error = err.Error()
		rec.Status = "eval-error"
		rec.Score, _ = ScoreCase(k, rec.Status, nil, nil)
		return rec
	}
	repo, home := filepath.Join(work, "repo"), filepath.Join(work, "state")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(capture, 0o755); err != nil {
		return fail(err)
	}
	target, head, err := buildRepo(repo, k)
	if err != nil {
		return fail(err)
	}
	rec.Target, rec.Candidate = target, head
	executable := o.Pi
	backends := map[string]any{"version": 2, "writable": map[string][]string{"reviewer": {capture}}}
	if o.Backend == "claude" {
		executable = o.Agent
	}
	backendsJSON, _ := json.Marshal(backends)
	if err := os.WriteFile(filepath.Join(home, "backends.json"), backendsJSON, 0o600); err != nil {
		return fail(err)
	}
	cfg, _ := json.Marshal(shimConfig{Pi: executable, Capture: capture})
	cfgPath := filepath.Join(work, "shim.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return fail(err)
	}
	native := filepath.Join(work, "bin")
	if err := os.MkdirAll(native, 0700); err != nil {
		return fail(err)
	}
	name := o.Backend
	if name == "" {
		name = "pi"
	}
	if err := os.Symlink(shimBin, filepath.Join(native, name)); err != nil {
		return fail(err)
	}
	a := axiClient{bin: bin, repo: repo, env: []string{"GREENHOUSE_HOME=" + home, shimEnv + "=" + cfgPath, "PATH=" + native + ":" + os.Getenv("PATH")}}

	defer stopDaemon(home)
	v, err := a.call(map[string]any{"op": "attach"})
	if err != nil {
		return fail(err)
	}
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": k.Rules, "reviewer_backend": o.Backend, "reviewer_model": o.Model, "fixer_backend": "pi", "fixer_model": "openai/gpt-5.6-luna"}
	if v, err = a.mutate(v, "apply", map[string]any{"settings": settings, "reason": "evaluation case " + k.ID}); err != nil {
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
	deadline := time.Now().Add(o.Settle)
	var review map[string]any
	for {
		v, err = a.call(map[string]any{"op": "inspect"})
		if err != nil {
			return fail(err)
		}
		run, _ := v["run"].(map[string]any)
		review, _ = run["review"].(map[string]any)
		if review != nil && review["status"] != "running" {
			break
		}
		if time.Now().After(deadline) {
			return fail(errors.New("review did not settle"))
		}
		time.Sleep(time.Second)
	}
	var x struct {
		Status      string   `json:"status"`
		Log         string   `json:"log"`
		Args        []string `json:"args"`
		Input       []byte   `json:"input"`
		InputDigest string   `json:"input_digest"`
		Result      *struct {
			Findings []Finding `json:"findings"`
		} `json:"result"`
	}
	b, _ := json.Marshal(review)
	if err := json.Unmarshal(b, &x); err != nil {
		return fail(err)
	}
	rec.Status, rec.Log, rec.Args, rec.InputDigest = x.Status, x.Log, x.Args, x.InputDigest
	delivered, err := os.ReadFile(filepath.Join(capture, "input.json"))
	rec.InputVerified = err == nil && bytes.Equal(delivered, x.Input)
	events, _ := os.ReadFile(filepath.Join(capture, "events.jsonl"))
	p := parseEvents(events)
	if o.Backend == "claude" {
		p = parseClaudeEvents(events)
	}
	if err := os.WriteFile(filepath.Join(capture, "events.jsonl"), credentialExpiry.ReplaceAll(compactEvents(events), []byte(`"credential_expires_at":"<redacted>"`)), 0o600); err != nil {
		return fail(err)
	}
	rec.ReportedProvider, rec.ReportedModel, rec.ThinkingLevel, rec.Usage = p.Provider, p.Model, p.Thinking, p.Usage
	rec.ModelAnswer = p.Answer
	rec.ModelFindings = modelFindings(p.Answer)
	var timing shimTiming
	if t, err := os.ReadFile(filepath.Join(capture, "timing.json")); err == nil && json.Unmarshal(t, &timing) == nil {
		rec.LatencyMillis = timing.Millis
	}
	rec.Findings = []Finding{}
	if x.Result != nil {
		rec.Findings = x.Result.Findings
	}
	rec.Score, _ = ScoreCase(k, rec.Status, rec.Findings, nil)
	return rec
}

func redactText(s string, secrets []string) string {
	for _, v := range secrets {
		s = strings.ReplaceAll(s, v, "<redacted>")
	}
	return s
}

// redactRecord removes credentials from the finding text before it is judged,
// so adjudication digests match the published report.
func redactRecord(rec *CaseRecord, secrets []string) {
	for i := range rec.Findings {
		rec.Findings[i].Explanation = redactText(rec.Findings[i].Explanation, secrets)
		rec.Findings[i].Action = redactText(rec.Findings[i].Action, secrets)
	}
	rec.ModelAnswer = redactText(rec.ModelAnswer, secrets)
	rec.ModelFindings = json.RawMessage(redactText(string(rec.ModelFindings), secrets))
}

func secretValues() []string {
	var out []string
	for _, v := range os.Environ() {
		name, value, _ := strings.Cut(v, "=")
		name = strings.ToUpper(name)
		if len(value) >= 8 && (strings.Contains(name, "KEY") || strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") || strings.Contains(name, "PASSWORD")) {
			out = append(out, value)
		}
	}
	return out
}

func redactTree(dir string) error {
	secrets := secretValues()
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		changed := b
		for _, s := range secrets {
			changed = bytes.ReplaceAll(changed, []byte(s), []byte("<redacted>"))
		}
		if !bytes.Equal(changed, b) {
			return os.WriteFile(p, changed, info.Mode())
		}
		return nil
	})
}

func runLive(o runOptions) (*Report, error) {
	if o.BaseURL != "" {
		return nil, errors.New("configure evaluation routing in native agent settings; -base-url is retired")
	}
	c, err := LoadCorpus(o.Corpus)
	if err != nil {
		return nil, err
	}
	cases, err := selectCases(c, o.Split, o.Cases)
	if err != nil {
		return nil, err
	}
	split, _ := normalizedSplit(o.Split)
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
	work, err := os.MkdirTemp("", "ge")
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
	agent, backend := o.Pi, "pi-json-v1"
	if o.Backend == "claude" {
		agent, backend = o.Agent, "claude-stream-json-v1"
	}
	piVersion, err := command("", nil, agent, "--version")
	if err != nil {
		return nil, err
	}
	version, err := command(root, nil, "git", "describe", "--always", "--dirty", "--abbrev=40")
	if err != nil {
		return nil, err
	}
	r := &Report{Config: RunConfig{Split: split, Label: o.Label, Started: time.Now().UTC(), Model: o.Model, Backend: backend, PiVersion: piVersion, BaseURL: o.BaseURL, GreenhouseVersion: version, GoVersion: runtime.Version(), CorpusVersion: c.Version, CorpusRevision: c.Revision, CorpusDigest: c.Digest, ScoredRevision: c.Revision, ScoredDigest: c.Digest, Parallel: o.Parallel, MaxSpan: maxSpan, BarHighRecall: barHighRecall}}
	records := make([]CaseRecord, len(cases))
	secrets := secretValues()
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.Parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				k := cases[i]
				records[i] = runCase(o, bin, shimBin, filepath.Join(work, fmt.Sprintf("h%02d", i)), k, filepath.Join(o.Out, "cases", k.ID))
				redactRecord(&records[i], secrets)
				fmt.Fprintf(os.Stderr, "%s: %s detected=%d/%d false_positives=%d %dms\n", k.ID, records[i].Status, records[i].Score.Detected, records[i].Score.Expected, records[i].Score.FalsePositives, records[i].LatencyMillis)
			}
		}()
	}
	for i := range cases {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	r.Config.Finished = time.Now().UTC()
	for _, rec := range records {
		r.Config.Cases = append(r.Config.Cases, rec.ID)
		r.CostUSD += rec.Usage.CostUSD
		r.LatencyMillis.Total += rec.LatencyMillis
		if rec.LatencyMillis > r.LatencyMillis.Max {
			r.LatencyMillis.Max = rec.LatencyMillis
		}
	}
	if len(records) > 0 {
		r.LatencyMillis.Mean = r.LatencyMillis.Total / int64(len(records))
	}
	r.Cases = records
	r.Summary = summarizeRecords(records)
	if o.JudgeModel != "" {
		a, err := judgeReport(r, c, judgeOptions{o.JudgeModel, o.Pi, o.BaseURL, o.Parallel})
		if err != nil {
			return nil, err
		}
		if err := rescoreReport(r, c, a); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(o.Out, 0o755); err != nil {
			return nil, err
		}
		if err := writeAdjudications(o.Out, a); err != nil {
			return nil, err
		}
	}
	if err := writeReport(o.Out, r); err != nil {
		return nil, err
	}
	return r, redactTree(o.Out)
}

func summarizeRecords(records []CaseRecord) Summary {
	var scores []CaseScore
	for _, rec := range records {
		scores = append(scores, rec.Score)
	}
	return Summarize(scores)
}

func rescoreReport(r *Report, c *Corpus, a *Adjudications) error {
	cases := map[string]Case{}
	for _, k := range c.Cases {
		cases[k.ID] = k
	}
	records := map[string][]Record{}
	if a != nil {
		for _, rec := range a.Records {
			records[rec.Case] = append(records[rec.Case], rec)
		}
	}
	for i := range r.Cases {
		rec := &r.Cases[i]
		k, ok := cases[rec.ID]
		if !ok {
			return fmt.Errorf("case %s not in corpus", rec.ID)
		}
		score, err := ScoreCase(k, rec.Status, rec.Findings, records[rec.ID])
		if err != nil {
			return err
		}
		rec.Score = score
		delete(records, rec.ID)
	}
	for id := range records {
		return fmt.Errorf("adjudication records name case %s, which the report lacks", id)
	}
	r.Config.ScoredRevision, r.Config.ScoredDigest = c.Revision, c.Digest
	r.Summary = summarizeRecords(r.Cases)
	return nil
}

func pct(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", *p*100)
}

func writeReport(dir string, r *Report) error {
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
	var md strings.Builder
	s := r.Summary
	fmt.Fprintf(&md, "# Review evaluation: %s\n\n", r.Config.Label)
	agent, pricing := "Pi "+r.Config.PiVersion, "Pi catalog pricing"
	if r.Config.Backend == "claude-stream-json-v1" {
		agent, pricing = "Claude Code "+strings.TrimSuffix(r.Config.PiVersion, " (Claude Code)"), "Claude Code list pricing"
	}
	fmt.Fprintf(&md, "- Model: `%s` through `%s`, %s\n", r.Config.Model, r.Config.Backend, agent)
	fmt.Fprintf(&md, "- Greenhouse: `%s`, corpus v%d revision %d `%s`\n", r.Config.GreenhouseVersion, r.Config.CorpusVersion, r.Config.CorpusRevision, r.Config.CorpusDigest)
	fmt.Fprintf(&md, "- Split: `%s`; counts by split: %v; counts by stratum: %v\n", r.Config.Split, s.BySplit, s.ByStratum)
	fmt.Fprintf(&md, "- Scored with corpus revision %d `%s`\n", r.Config.ScoredRevision, r.Config.ScoredDigest)
	fmt.Fprintf(&md, "- Started %s, finished %s, parallel %d\n\n", r.Config.Started.Format(time.RFC3339), r.Config.Finished.Format(time.RFC3339), r.Config.Parallel)
	bar := "not met"
	if s.BarMet {
		bar = "met"
	}
	fmt.Fprintf(&md, "Bar (high-severity recall >= %.0f%%, no blocking result on clean cases): **%s**\n\n", r.Config.BarHighRecall*100, bar)
	fmt.Fprintf(&md, "| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&md, "| High-severity recall | %s (%d/%d) |\n", pct(s.HighRecall), s.HighDetected, s.HighExpected)
	fmt.Fprintf(&md, "| Recall | %s (%d/%d) |\n", pct(s.Recall), s.Detected, s.Expected)
	fmt.Fprintf(&md, "| Precision | %s (%d/%d) |\n", pct(s.Precision), s.Matched, s.Findings-s.Allowed)
	fmt.Fprintf(&md, "| Allowed judgment findings (excluded from precision) | %d |\n", s.Allowed)
	fmt.Fprintf(&md, "| Priority accuracy | %s (%d/%d) |\n", pct(s.PriorityAccuracy), s.PriorityCorrect, s.PriorityJudged)
	fmt.Fprintf(&md, "| False positives (clean / noise / all) | %d / %d / %d |\n", s.CleanFalsePositives, s.NoiseFalsePositives, s.FalsePositives)
	fmt.Fprintf(&md, "| False positives judged a different defect at an expected location | %d |\n", s.WrongDefect)
	fmt.Fprintf(&md, "| Eligible findings without an adjudication record (not credited) | %d |\n", s.Unadjudicated)
	fmt.Fprintf(&md, "| Blocking results on clean cases | %d |\n", s.CleanBlocking)
	fmt.Fprintf(&md, "| Inconclusive reviews | %d/%d |\n", s.Inconclusive, s.Cases)
	fmt.Fprintf(&md, "| Cost (%s) | $%.4f |\n", pricing, r.CostUSD)
	fmt.Fprintf(&md, "| Latency total / mean / max | %.1fs / %.1fs / %.1fs |\n\n", float64(r.LatencyMillis.Total)/1000, float64(r.LatencyMillis.Mean)/1000, float64(r.LatencyMillis.Max)/1000)
	fmt.Fprintf(&md, "| Case | Status | Detected | False positives | Blocking | Priority | Cost | Latency |\n|---|---|---|---|---|---|---|---|\n")
	recs := append([]CaseRecord(nil), r.Cases...)
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	for _, c := range recs {
		fmt.Fprintf(&md, "| %s | %s | %d/%d | %d | %d | %d/%d | $%.4f | %.1fs |\n", c.ID, c.Status, c.Score.Detected, c.Score.Expected, c.Score.FalsePositives, c.Score.Blocking, c.Score.PriorityCorrect, c.Score.PriorityJudged, c.Usage.CostUSD, float64(c.LatencyMillis)/1000)
	}
	fmt.Fprintf(&md, "\nPer-case `input.json` holds the exact bytes Greenhouse delivered to the reviewer on stdin, `argv.json` the exact arguments, and `events.jsonl` the reviewer's raw JSON event stream through `%s`.\n", r.Config.Backend)
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(md.String()), 0o644)
}
