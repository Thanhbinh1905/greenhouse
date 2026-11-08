package main

import "strings"

// maxSpan is the longest finding range, in lines, that can match an expected
// finding. Wider ranges do not locate a defect.
const maxSpan = 15

const barHighRecall = 0.8

type Finding struct {
	Priority    string `json:"priority"`
	Origin      string `json:"origin"`
	File        string `json:"file"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Rule        string `json:"rule"`
	Explanation string `json:"explanation"`
	Action      string `json:"action"`
}

// Adjudication records why one finding scored as it did. Expected is the index
// of the expected finding it detected or duplicated, or -1.
type Adjudication struct {
	Finding  int    `json:"finding"`
	Expected int    `json:"expected"`
	Verdict  string `json:"verdict"`
	Keyword  string `json:"keyword,omitempty"`
}

type CaseScore struct {
	ID              string         `json:"id"`
	Category        string         `json:"category"`
	Status          string         `json:"status"`
	Inconclusive    bool           `json:"inconclusive"`
	Expected        int            `json:"expected"`
	Detected        int            `json:"detected"`
	HighExpected    int            `json:"high_expected"`
	HighDetected    int            `json:"high_detected"`
	Findings        int            `json:"findings"`
	Matched         int            `json:"matched"`
	Duplicates      int            `json:"duplicates"`
	FalsePositives  int            `json:"false_positives"`
	WrongDefect     int            `json:"wrong_defect"`
	NoiseFlagged    int            `json:"noise_flagged"`
	Blocking        int            `json:"blocking"`
	PriorityJudged  int            `json:"priority_judged"`
	PriorityCorrect int            `json:"priority_correct"`
	Missed          []Expected     `json:"missed"`
	Unmatched       []Finding      `json:"unmatched"`
	Adjudications   []Adjudication `json:"adjudications"`
}

type Summary struct {
	Cases               int      `json:"cases"`
	Inconclusive        int      `json:"inconclusive"`
	Expected            int      `json:"expected"`
	Detected            int      `json:"detected"`
	HighExpected        int      `json:"high_expected"`
	HighDetected        int      `json:"high_detected"`
	Findings            int      `json:"findings"`
	Matched             int      `json:"matched"`
	FalsePositives      int      `json:"false_positives"`
	WrongDefect         int      `json:"wrong_defect"`
	CleanFalsePositives int      `json:"clean_false_positives"`
	CleanBlocking       int      `json:"clean_blocking"`
	NoiseFalsePositives int      `json:"noise_false_positives"`
	PriorityJudged      int      `json:"priority_judged"`
	PriorityCorrect     int      `json:"priority_correct"`
	Recall              *float64 `json:"recall"`
	HighRecall          *float64 `json:"high_recall"`
	Precision           *float64 `json:"precision"`
	PriorityAccuracy    *float64 `json:"priority_accuracy"`
	BarMet              bool     `json:"bar_met"`
}

func blocking(priority string) bool { return priority == "P0" || priority == "P1" }

func locates(e Expected, f Finding) bool {
	return f.File == e.File && f.Origin == e.Origin && f.Rule == e.Rule && f.Start <= e.End && f.End >= e.Start && f.End-f.Start < maxSpan
}

func names(e Expected, f Finding) string {
	text := strings.ToLower(f.Explanation + "\n" + f.Action)
	for _, k := range e.Keywords {
		if strings.Contains(text, k) {
			return k
		}
	}
	return ""
}

// ScoreCase compares the findings of one completed review with the case's
// ground truth. A finding detects an expected finding only when it is at the
// expected location and names the expected defect. Each expected finding is
// detected once; later findings naming the same defect count as duplicates.
// A finding at an expected location that names another defect is a
// wrong-defect false positive.
func ScoreCase(k Case, status string, findings []Finding) CaseScore {
	s := CaseScore{ID: k.ID, Category: k.Category, Status: status, Inconclusive: status != "clean-review" && status != "outstanding-findings", Expected: len(k.Expected), Findings: len(findings), Missed: []Expected{}, Unmatched: []Finding{}, Adjudications: []Adjudication{}}
	detected := make([]bool, len(k.Expected))
	for n, f := range findings {
		hit, dup, located, keyword := -1, -1, false, ""
		for i, e := range k.Expected {
			if !locates(e, f) {
				continue
			}
			located = true
			kw := names(e, f)
			if kw == "" {
				continue
			}
			if !detected[i] {
				hit, keyword = i, kw
				break
			}
			if dup < 0 {
				dup, keyword = i, kw
			}
		}
		switch {
		case hit >= 0:
			s.Adjudications = append(s.Adjudications, Adjudication{n, hit, "detected", keyword})
			detected[hit] = true
			s.Matched++
			s.PriorityJudged++
			for _, p := range k.Expected[hit].Priorities {
				if p == f.Priority {
					s.PriorityCorrect++
				}
			}
		case dup >= 0:
			s.Adjudications = append(s.Adjudications, Adjudication{n, dup, "duplicate", keyword})
			s.Matched++
			s.Duplicates++
		default:
			verdict := "false-positive"
			if located {
				verdict = "wrong-defect"
				s.WrongDefect++
			}
			s.Adjudications = append(s.Adjudications, Adjudication{n, -1, verdict, ""})
			s.FalsePositives++
			s.Unmatched = append(s.Unmatched, f)
			if blocking(f.Priority) {
				s.Blocking++
			}
			for _, n := range k.Noise {
				if f.File == n.File && f.Start <= n.End && f.End >= n.Start {
					s.NoiseFlagged++
					break
				}
			}
		}
	}
	for i, e := range k.Expected {
		if e.Severity == "high" {
			s.HighExpected++
		}
		if detected[i] {
			s.Detected++
			if e.Severity == "high" {
				s.HighDetected++
			}
		} else {
			s.Missed = append(s.Missed, e)
		}
	}
	if s.Inconclusive && k.Category == "clean" {
		s.Blocking++
	}
	return s
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	r := float64(n) / float64(d)
	return &r
}

func Summarize(scores []CaseScore) Summary {
	m := Summary{Cases: len(scores)}
	for _, s := range scores {
		if s.Inconclusive {
			m.Inconclusive++
		}
		m.Expected += s.Expected
		m.Detected += s.Detected
		m.HighExpected += s.HighExpected
		m.HighDetected += s.HighDetected
		m.Findings += s.Findings
		m.Matched += s.Matched
		m.FalsePositives += s.FalsePositives
		m.WrongDefect += s.WrongDefect
		m.PriorityJudged += s.PriorityJudged
		m.PriorityCorrect += s.PriorityCorrect
		switch s.Category {
		case "clean":
			m.CleanFalsePositives += s.FalsePositives
			m.CleanBlocking += s.Blocking
		case "noise":
			m.NoiseFalsePositives += s.FalsePositives
		}
	}
	m.Recall = ratio(m.Detected, m.Expected)
	m.HighRecall = ratio(m.HighDetected, m.HighExpected)
	m.Precision = ratio(m.Matched, m.Findings)
	m.PriorityAccuracy = ratio(m.PriorityCorrect, m.PriorityJudged)
	m.BarMet = m.HighRecall != nil && *m.HighRecall >= barHighRecall && m.CleanBlocking == 0
	return m
}
