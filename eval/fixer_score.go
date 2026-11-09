package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const barFixedRate = 0.75

type FixScore struct {
	Response        string   `json:"response"`
	ResponseCorrect bool     `json:"response_correct"`
	AcceptPassed    int      `json:"accept_passed"`
	AcceptTotal     int      `json:"accept_total"`
	Regressions     []string `json:"regressions"`
	ScopeViolations []string `json:"scope_violations"`
	Fixed           bool     `json:"fixed"`
}

type FixSummary struct {
	Cases            int      `json:"cases"`
	Fixed            int      `json:"fixed"`
	FixedRate        *float64 `json:"fixed_rate"`
	AcceptPassed     int      `json:"accept_passed"`
	AcceptTotal      int      `json:"accept_total"`
	RegressionCases  int      `json:"regression_cases"`
	Regressions      int      `json:"regressions"`
	ScopeCases       int      `json:"scope_violation_cases"`
	ScopeViolations  int      `json:"scope_violations"`
	ResponsesCorrect int      `json:"responses_correct"`
	BarMet           bool     `json:"bar_met"`
}

type TestOutcomes map[string]string

// runTests writes tree and the hidden tests into dir and runs every package's
// tests. A test that never reported a result, for example because its
// package did not build, counts as failed.
func runTests(dir string, tree, hidden map[string]string) (TestOutcomes, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	if err := writeTree(dir, tree); err != nil {
		return nil, "", err
	}
	if err := writeTree(dir, hidden); err != nil {
		return nil, "", err
	}
	c := exec.Command("go", "test", "-json", "-count=1", "./...")
	c.Dir = dir
	c.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local")
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	c.Run()
	results := TestOutcomes{}
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 4096), 4*1024*1024)
	var output strings.Builder
	for sc.Scan() {
		var ev struct {
			Action string `json:"Action"`
			Test   string `json:"Test"`
			Output string `json:"Output"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		output.WriteString(ev.Output)
		if ev.Test != "" && !strings.Contains(ev.Test, "/") && (ev.Action == "pass" || ev.Action == "fail") {
			results[ev.Test] = ev.Action
		}
	}
	output.WriteString(errb.String())
	for _, body := range []map[string]string{tree, hidden} {
		for name, src := range body {
			if !strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, m := range hiddenTestName.FindAllStringSubmatch(src, -1) {
				if _, ok := results[m[1]]; !ok {
					results[m[1]] = runOneTest(dir, m[1])
				}
			}
		}
	}
	return results, output.String(), nil
}

// runOneTest reruns a test that reported nothing because another test in its
// package panicked or timed out.
func runOneTest(dir, name string) string {
	c := exec.Command("go", "test", "-count=1", "-run", "^"+name+"$", "./...")
	c.Dir = dir
	c.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local")
	if c.Run() != nil {
		return "fail"
	}
	return "pass"
}

// changedLines aligns two versions of a file by longest common subsequence.
// It returns the old line numbers that were removed or modified and, for each
// line inserted without replacing one, the old line after which it was
// inserted (0 for the top).
func changedLines(old, new string) (removed, insertedAfter []int) {
	a, b := splitLines(old), splitLines(new)
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	replacing := false
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			i++
			j++
			replacing = false
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			removed = append(removed, i+1)
			i++
			replacing = true
		default:
			if !replacing {
				insertedAfter = append(insertedAfter, i)
			}
			j++
		}
	}
	return removed, insertedAfter
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// scopeViolations lists edits outside the case scope. A removed or modified
// line must lie inside a scope range; an inserted line must border one.
func scopeViolations(head, repaired map[string]string, scope []LineRange) []string {
	inScope := func(file string, line int) bool {
		for _, r := range scope {
			if r.File == file && line >= r.Start && line <= r.End {
				return true
			}
		}
		return false
	}
	names := map[string]bool{}
	for name := range head {
		names[name] = true
	}
	for name := range repaired {
		names[name] = true
	}
	var out []string
	for _, name := range sortedKeys(names) {
		before, inHead := head[name]
		after, inRepair := repaired[name]
		switch {
		case before == after && inHead == inRepair:
		case !inHead:
			out = append(out, "added "+name)
		case !inRepair:
			out = append(out, "deleted "+name)
		default:
			removed, inserted := changedLines(before, after)
			for _, line := range removed {
				if !inScope(name, line) {
					out = append(out, fmt.Sprintf("changed %s:%d", name, line))
				}
			}
			for _, line := range inserted {
				if !inScope(name, line) && !inScope(name, line+1) {
					out = append(out, fmt.Sprintf("inserted after %s:%d", name, line))
				}
			}
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ScoreFix scores one repair. responses maps each selected finding label to
// the fixer's answer; refused records that the product refused an edit
// outside the authorized paths.
func ScoreFix(k FixCase, responses map[string]string, repaired map[string]string, refused bool, tests TestOutcomes) FixScore {
	s := FixScore{Regressions: []string{}, ScopeViolations: scopeViolations(k.Head, repaired, k.Scope)}
	if refused {
		s.ScopeViolations = append(s.ScopeViolations, "the fixer edited a path outside the authorized paths; Greenhouse refused the repair")
	}
	var answers []string
	s.ResponseCorrect = true
	for _, label := range k.selectedLabels() {
		r := responses[label]
		if r == "" {
			r = "none"
		}
		answers = append(answers, r)
		if r != k.ExpectedResponse {
			s.ResponseCorrect = false
		}
	}
	s.Response = strings.Join(answers, ",")
	for _, name := range sortedTests(tests) {
		pass := tests[name] == "pass"
		if strings.HasPrefix(name, "TestAccept") {
			s.AcceptTotal++
			if pass {
				s.AcceptPassed++
			}
		} else if !pass {
			s.Regressions = append(s.Regressions, name)
		}
	}
	s.Fixed = s.ResponseCorrect && s.AcceptTotal > 0 && s.AcceptPassed == s.AcceptTotal && len(s.Regressions) == 0 && len(s.ScopeViolations) == 0
	return s
}

func sortedTests(t TestOutcomes) []string {
	out := make([]string, 0, len(t))
	for k := range t {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func SummarizeFix(scores []FixScore) FixSummary {
	s := FixSummary{Cases: len(scores)}
	for _, c := range scores {
		if c.Fixed {
			s.Fixed++
		}
		if c.ResponseCorrect {
			s.ResponsesCorrect++
		}
		s.AcceptPassed += c.AcceptPassed
		s.AcceptTotal += c.AcceptTotal
		if len(c.Regressions) > 0 {
			s.RegressionCases++
			s.Regressions += len(c.Regressions)
		}
		if len(c.ScopeViolations) > 0 {
			s.ScopeCases++
			s.ScopeViolations += len(c.ScopeViolations)
		}
	}
	if s.Cases > 0 {
		r := float64(s.Fixed) / float64(s.Cases)
		s.FixedRate = &r
	}
	s.BarMet = s.FixedRate != nil && *s.FixedRate >= barFixedRate && s.Regressions == 0 && s.ScopeViolations == 0
	return s
}
