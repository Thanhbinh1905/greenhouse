package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const judgeSystem = "You are an evaluation judge. Decide whether one code review finding identifies one specific planted defect. Judge only the defect named, not the location. Return only the requested JSON."

const judgeInstructions = `Input is JSON with "expected", the planted defects at the finding's location (id, file, start, end, defect), and "finding", one reviewer finding (file, start, end, priority, explanation, action).
Return only this JSON object: {"expected":"<id from expected>","verdict":"detects|different-defect","rationale":"<one sentence>"}.
Use "detects" only when the finding describes the same root cause as that planted defect. A finding that shares identifiers, lines or wording but describes another root cause is "different-defect"; then name the closest expected id.
The rationale is one sentence that quotes at least 12 consecutive characters of the finding's explanation or action verbatim inside double quotes.`

type judgeOptions struct {
	Model, Pi, BaseURL string
	Parallel           int
}

type judgeVerdict struct {
	Expected  string `json:"expected"`
	Verdict   string `json:"verdict"`
	Rationale string `json:"rationale"`
}

type judgeTask struct {
	caseID   string
	finding  int
	f        Finding
	eligible []Expected
}

func judgeOnce(o judgeOptions, t judgeTask) (judgeVerdict, Usage, error) {
	var v judgeVerdict
	dir, err := os.MkdirTemp("", "judge-")
	if err != nil {
		return v, Usage{}, err
	}
	defer os.RemoveAll(dir)
	if o.BaseURL != "" {
		return v, Usage{}, fmt.Errorf("configure judge routing in native Pi settings; -base-url is retired")
	}
	type expected struct {
		ID     string `json:"id"`
		File   string `json:"file"`
		Start  int    `json:"start"`
		End    int    `json:"end"`
		Defect string `json:"defect"`
	}
	in := struct {
		Instructions string     `json:"instructions"`
		Expected     []expected `json:"expected"`
		Finding      Finding    `json:"finding"`
	}{Instructions: judgeInstructions, Finding: t.f}
	for _, e := range t.eligible {
		in.Expected = append(in.Expected, expected{e.ID, e.File, e.Start, e.End, e.Defect})
	}
	input, _ := json.Marshal(in)
	cmd := exec.Command(o.Pi, "--print", "--mode", "json", "--no-session", "--no-tools", "--no-extensions", "--no-mcp", "--no-skills", "--no-context-files", "--no-prompt-templates", "--no-themes", "--no-approve", "--offline", "--model", o.Model, "--system-prompt", judgeSystem)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if (!strings.HasPrefix(kv, "PI_") || strings.HasPrefix(kv, "PI_CODING_AGENT_DIR=")) && !strings.HasPrefix(kv, shimEnv+"=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	p := parseEvents(out)
	if err != nil {
		return v, p.Usage, fmt.Errorf("judge process: %w", err)
	}
	if err := json.Unmarshal([]byte(p.Answer), &v); err != nil {
		return v, p.Usage, fmt.Errorf("judge answer is not JSON: %q", p.Answer)
	}
	return v, p.Usage, nil
}

func validVerdict(t judgeTask, v judgeVerdict) bool {
	if v.Verdict != "detects" && v.Verdict != "different-defect" || !cites(v.Rationale, t.f) {
		return false
	}
	for _, e := range t.eligible {
		if e.ID == v.Expected {
			return true
		}
	}
	return false
}

// judgeReport asks a judge model, one fresh Pi process per finding, whether
// each eligible finding identifies its planted defect. A finding whose judge
// answer stays invalid after one retry gets no record and earns no credit.
func judgeReport(r *Report, c *Corpus, o judgeOptions) (*Adjudications, error) {
	cases := map[string]Case{}
	for _, k := range c.Cases {
		cases[k.ID] = k
	}
	var tasks []judgeTask
	for _, rec := range r.Cases {
		k, ok := cases[rec.ID]
		if !ok {
			return nil, fmt.Errorf("case %s not in corpus", rec.ID)
		}
		for n, f := range rec.Findings {
			t := judgeTask{caseID: rec.ID, finding: n, f: f}
			for _, e := range k.Expected {
				if locates(e, f) {
					t.eligible = append(t.eligible, e)
				}
			}
			if len(t.eligible) > 0 {
				tasks = append(tasks, t)
			}
		}
	}
	piVersion, err := command("", nil, o.Pi, "--version")
	if err != nil {
		return nil, err
	}
	verdicts := make([]*judgeVerdict, len(tasks))
	var mu sync.Mutex
	var cost float64
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.Parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				for attempt := 0; attempt < 2; attempt++ {
					v, u, err := judgeOnce(o, tasks[i])
					mu.Lock()
					cost += u.CostUSD
					mu.Unlock()
					if err == nil && validVerdict(tasks[i], v) {
						verdicts[i] = &v
						break
					}
					fmt.Fprintf(os.Stderr, "judge %s finding %d attempt %d invalid: %v %+v\n", tasks[i].caseID, tasks[i].finding, attempt+1, err, v)
				}
			}
		}()
	}
	for i := range tasks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	a := &Adjudications{Schema: adjudicationSchema, Judge: Judge{Kind: "model", Model: o.Model, Backend: "pi --print --mode json --no-session --no-tools", PiVersion: piVersion, BaseURL: o.BaseURL, Prompt: "system: " + judgeSystem + "\ninstructions: " + judgeInstructions}, Records: []Record{}}
	detected := map[string]bool{}
	skipped := 0
	for i, t := range tasks {
		v := verdicts[i]
		if v == nil {
			skipped++
			continue
		}
		verdict := v.Verdict
		key := t.caseID + "/" + v.Expected
		if verdict == "detects" {
			if detected[key] {
				verdict = "duplicate"
			}
			detected[key] = true
		}
		a.Records = append(a.Records, Record{t.caseID, t.finding, findingDigest(t.f), v.Expected, verdict, v.Rationale})
	}
	a.Judge.Note = fmt.Sprintf("%d eligible findings, %d without a valid judge answer; judge cost $%.4f at Pi catalog prices", len(tasks), skipped, cost)
	return a, nil
}
