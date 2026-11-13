package eval

import "fmt"

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

// Adjudication is the scored outcome of one finding. Expected is the ID of the
// expected finding a record judged it against, or "".
type Adjudication struct {
	Finding   int    `json:"finding"`
	Expected  string `json:"expected"`
	Verdict   string `json:"verdict"`
	Rationale string `json:"rationale,omitempty"`
}

type CaseScore struct {
	Split           string         `json:"split"`
	Stratum         string         `json:"stratum"`
	Allowed         int            `json:"allowed"`
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
	Unadjudicated   int            `json:"unadjudicated"`
	NoiseFlagged    int            `json:"noise_flagged"`
	Blocking        int            `json:"blocking"`
	PriorityJudged  int            `json:"priority_judged"`
	PriorityCorrect int            `json:"priority_correct"`
	Missed          []Expected     `json:"missed"`
	Unmatched       []Finding      `json:"unmatched"`
	Adjudications   []Adjudication `json:"adjudications"`
}

type Summary struct {
	Allowed             int            `json:"allowed"`
	BySplit             map[string]int `json:"by_split"`
	ByStratum           map[string]int `json:"by_stratum"`
	Cases               int            `json:"cases"`
	Inconclusive        int            `json:"inconclusive"`
	Expected            int            `json:"expected"`
	Detected            int            `json:"detected"`
	HighExpected        int            `json:"high_expected"`
	HighDetected        int            `json:"high_detected"`
	Findings            int            `json:"findings"`
	Matched             int            `json:"matched"`
	FalsePositives      int            `json:"false_positives"`
	WrongDefect         int            `json:"wrong_defect"`
	Unadjudicated       int            `json:"unadjudicated"`
	CleanFalsePositives int            `json:"clean_false_positives"`
	CleanBlocking       int            `json:"clean_blocking"`
	NoiseFalsePositives int            `json:"noise_false_positives"`
	PriorityJudged      int            `json:"priority_judged"`
	PriorityCorrect     int            `json:"priority_correct"`
	Recall              *float64       `json:"recall"`
	HighRecall          *float64       `json:"high_recall"`
	Precision           *float64       `json:"precision"`
	PriorityAccuracy    *float64       `json:"priority_accuracy"`
	BarMet              bool           `json:"bar_met"`
}

func blocking(priority string) bool { return priority == "P0" || priority == "P1" }

func locates(e Expected, f Finding) bool {
	return f.File == e.File && f.Origin == e.Origin && f.Rule == e.Rule && f.Start <= e.End && f.End >= e.Start && f.End-f.Start < maxSpan
}

// ScoreCase compares the findings of one completed review with the case's
// ground truth. Location, file, origin and rule only make a finding eligible;
// credit comes from the case's adjudication records. An eligible finding
// without a record earns no credit. Records that contradict the findings or
// each other are refused.
func ScoreCase(k Case, status string, findings []Finding, records []Record) (CaseScore, error) {
	s := CaseScore{ID: k.ID, Split: k.Split, Stratum: k.Stratum, Category: k.Category, Status: status, Inconclusive: status != "clean-review" && status != "outstanding-findings", Expected: len(k.Expected), Findings: len(findings), Missed: []Expected{}, Unmatched: []Finding{}, Adjudications: []Adjudication{}}
	byFinding := map[int]Record{}
	for _, r := range records {
		if r.Case != k.ID || r.Finding < 0 || r.Finding >= len(findings) {
			return s, fmt.Errorf("record for %s finding %d does not name a recorded finding of %s", r.Case, r.Finding, k.ID)
		}
		if _, dup := byFinding[r.Finding]; dup {
			return s, fmt.Errorf("%s finding %d has two records", k.ID, r.Finding)
		}
		if r.Verdict != "detects" && r.Verdict != "different-defect" && r.Verdict != "duplicate" {
			return s, fmt.Errorf("%s finding %d: unknown verdict %q", k.ID, r.Finding, r.Verdict)
		}
		byFinding[r.Finding] = r
	}
	detected := make([]bool, len(k.Expected))
	for n, f := range findings {
		eligible := map[string]int{}
		for i, e := range k.Expected {
			if locates(e, f) {
				eligible[e.ID] = i
			}
		}

		allowed := false
		if len(eligible) == 0 && k.Category == "judgment" {
			for _, a := range k.Allowed {
				if f.Start >= 1 && f.End >= f.Start && locates(Expected{File: a.File, Start: a.Start, End: a.End, Origin: a.Origin, Rule: a.Rule}, f) {
					allowed = true
					break
				}
			}
		}
		if allowed {
			if _, ok := byFinding[n]; ok {
				return s, fmt.Errorf("%s finding %d: allowed finding cannot carry detection record", k.ID, n)
			}
			s.Allowed++
			s.Adjudications = append(s.Adjudications, Adjudication{Finding: n, Verdict: "allowed"})
			continue
		}
		r, judged := byFinding[n]
		credit := -1
		switch {
		case len(eligible) == 0:
			if judged {
				return s, fmt.Errorf("%s finding %d is not at any expected location but has a record", k.ID, n)
			}
			s.Adjudications = append(s.Adjudications, Adjudication{Finding: n, Verdict: "false-positive"})
		case !judged:
			s.Unadjudicated++
			s.Adjudications = append(s.Adjudications, Adjudication{Finding: n, Verdict: "unadjudicated"})
		default:
			i, ok := eligible[r.Expected]
			switch {
			case r.FindingDigest != findingDigest(f):
				return s, fmt.Errorf("%s finding %d changed since it was adjudicated", k.ID, n)
			case !cites(r.Rationale, f):
				return s, fmt.Errorf("%s finding %d: rationale does not quote the finding", k.ID, n)
			case !ok:
				return s, fmt.Errorf("%s finding %d is not at the location of expected %q", k.ID, n, r.Expected)
			case r.Verdict == "detects" && detected[i]:
				return s, fmt.Errorf("%s finding %d detects %q, already detected; record a duplicate", k.ID, n, r.Expected)
			case r.Verdict == "duplicate" && !detected[i]:
				return s, fmt.Errorf("%s finding %d duplicates %q before any detection", k.ID, n, r.Expected)
			}
			s.Adjudications = append(s.Adjudications, Adjudication{n, r.Expected, r.Verdict, r.Rationale})
			credit = i
		}
		switch {
		case credit >= 0 && r.Verdict == "detects":
			detected[credit] = true
			s.Matched++
			s.PriorityJudged++
			for _, p := range k.Expected[credit].Priorities {
				if p == f.Priority {
					s.PriorityCorrect++
				}
			}
		case credit >= 0 && r.Verdict == "duplicate":
			s.Matched++
			s.Duplicates++
		default:
			if credit >= 0 {
				s.WrongDefect++
			}
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
	return s, nil
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	r := float64(n) / float64(d)
	return &r
}

func Summarize(scores []CaseScore) Summary {
	m := Summary{Cases: len(scores), BySplit: map[string]int{}, ByStratum: map[string]int{}}
	for _, s := range scores {
		if s.Inconclusive {
			m.Inconclusive++
		}
		m.Expected += s.Expected
		m.Detected += s.Detected
		m.HighExpected += s.HighExpected
		m.HighDetected += s.HighDetected
		m.Allowed += s.Allowed
		m.BySplit[s.Split]++
		m.ByStratum[s.Stratum]++
		m.Findings += s.Findings
		m.Matched += s.Matched
		m.FalsePositives += s.FalsePositives
		m.WrongDefect += s.WrongDefect
		m.Unadjudicated += s.Unadjudicated
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
	m.Precision = ratio(m.Matched, m.Findings-m.Allowed)
	m.PriorityAccuracy = ratio(m.PriorityCorrect, m.PriorityJudged)
	m.BarMet = m.HighRecall != nil && *m.HighRecall >= barHighRecall && m.CleanBlocking == 0
	return m
}
