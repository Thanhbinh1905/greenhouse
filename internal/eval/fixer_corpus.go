package eval

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type FixCorpus struct {
	Version  int       `json:"version"`
	Revision int       `json:"revision"`
	Digest   string    `json:"digest"`
	Cases    []FixCase `json:"-"`
}

type FixCase struct {
	Version          int               `json:"version"`
	ID               string            `json:"id"`
	Description      string            `json:"description"`
	Intent           string            `json:"intent"`
	Rules            []string          `json:"rules"`
	Findings         []SeededFinding   `json:"findings"`
	ExpectedResponse string            `json:"expected_response"`
	Scope            []LineRange       `json:"scope"`
	Base             map[string]string `json:"-"`
	Head             map[string]string `json:"-"`
	Fix              map[string]string `json:"-"`
	Hidden           map[string]string `json:"-"`
}

// SeededFinding is a finding the evaluation's stand-in reviewer reports, so
// every run repairs the same review.
type SeededFinding struct {
	Label       string `json:"label"`
	Selected    bool   `json:"selected"`
	Priority    string `json:"priority"`
	Origin      string `json:"origin"`
	Rule        string `json:"rule"`
	File        string `json:"file"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Explanation string `json:"explanation"`
	Action      string `json:"action"`
}

type LineRange struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

var hiddenTestName = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

func LoadFixCorpus(root string) (*FixCorpus, error) {
	b, err := os.ReadFile(filepath.Join(root, "corpus.json"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Version  int      `json:"version"`
		Revision int      `json:"revision"`
		Cases    []string `json:"cases"`
	}
	if err := strictDecode(b, &manifest); err != nil {
		return nil, fmt.Errorf("corpus.json: %w", err)
	}
	if manifest.Version != 1 {
		return nil, errors.New("corpus.json: unsupported version")
	}
	listed := map[string]bool{}
	for _, id := range manifest.Cases {
		listed[id] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && !listed[e.Name()] {
			return nil, fmt.Errorf("case directory %s not listed in corpus.json", e.Name())
		}
	}
	c := &FixCorpus{Version: manifest.Version, Revision: manifest.Revision}
	for _, id := range manifest.Cases {
		k, err := loadFixCase(filepath.Join(root, id), id)
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", id, err)
		}
		c.Cases = append(c.Cases, *k)
	}
	c.Digest, err = treeDigest(root)
	return c, err
}

func loadFixCase(dir, id string) (*FixCase, error) {
	b, err := os.ReadFile(filepath.Join(dir, "case.json"))
	if err != nil {
		return nil, err
	}
	var k FixCase
	if err := strictDecode(b, &k); err != nil {
		return nil, err
	}
	if k.Version != 1 || k.ID != id || strings.TrimSpace(k.Intent) == "" {
		return nil, errors.New("case needs version 1, an id matching its directory and an intent")
	}
	for name, p := range map[string]*map[string]string{"base": &k.Base, "head": &k.Head, "fix": &k.Fix, "hidden": &k.Hidden} {
		if *p, err = readTree(filepath.Join(dir, name)); err != nil {
			return nil, err
		}
	}
	for _, r := range k.Rules {
		if _, ok := builtinRules[r]; !ok {
			return nil, fmt.Errorf("unsupported rule selection %q", r)
		}
	}
	selected := 0
	labels := map[string]bool{}
	for _, f := range k.Findings {
		if err := checkRange(k.Head, f.File, f.Start, f.End); err != nil {
			return nil, err
		}
		if !expectedID.MatchString(f.Label) || labels[f.Label] {
			return nil, fmt.Errorf("finding label %q must be unique and match %s", f.Label, expectedID)
		}
		labels[f.Label] = true
		if !validPriority(f.Priority) || f.Origin == "policy" && f.Rule == "" || f.Origin == "correctness" && f.Rule != "" || f.Origin != "policy" && f.Origin != "correctness" {
			return nil, fmt.Errorf("finding %s needs a priority and a valid origin/rule pair", f.Label)
		}
		if strings.TrimSpace(f.Explanation) == "" || strings.TrimSpace(f.Action) == "" {
			return nil, fmt.Errorf("finding %s needs an explanation and an action", f.Label)
		}
		if f.Selected {
			if f.Priority == "P3" {
				return nil, fmt.Errorf("finding %s is informational and cannot be selected", f.Label)
			}
			selected++
		}
	}
	if selected == 0 {
		return nil, errors.New("select at least one finding")
	}
	switch k.ExpectedResponse {
	case "fixed":
		if len(k.Fix) == 0 {
			return nil, errors.New("a fixed case needs a reference repair in fix/")
		}
	case "disputed":
		if len(k.Fix) != 0 {
			return nil, errors.New("a disputed case has no reference repair")
		}
	default:
		return nil, fmt.Errorf("expected_response %q must be fixed or disputed", k.ExpectedResponse)
	}
	for name := range k.Fix {
		if _, ok := k.Head[name]; !ok {
			return nil, fmt.Errorf("reference repair adds %s, which the head lacks", name)
		}
	}
	if len(k.Scope) == 0 {
		return nil, errors.New("scope required")
	}
	for _, r := range k.Scope {
		if err := checkRange(k.Head, r.File, r.Start, r.End); err != nil {
			return nil, err
		}
	}
	var accept, regress int
	for name, body := range k.Hidden {
		if !strings.HasSuffix(name, "_test.go") {
			return nil, fmt.Errorf("hidden file %s is not a Go test file", name)
		}
		if _, ok := k.Head[name]; ok {
			return nil, fmt.Errorf("hidden test %s would replace a candidate file", name)
		}
		for _, m := range hiddenTestName.FindAllStringSubmatch(body, -1) {
			switch {
			case strings.HasPrefix(m[1], "TestAccept"):
				accept++
			case strings.HasPrefix(m[1], "TestRegress"):
				regress++
			default:
				return nil, fmt.Errorf("hidden test %s must start with TestAccept or TestRegress", m[1])
			}
		}
	}
	if accept == 0 || regress == 0 {
		return nil, errors.New("hidden tests need at least one TestAccept and one TestRegress function")
	}
	return &k, nil
}

func (k FixCase) repaired() map[string]string {
	out := maps.Clone(k.Head)
	maps.Copy(out, k.Fix)
	return out
}

func (k FixCase) selectedLabels() []string {
	var out []string
	for _, f := range k.Findings {
		if f.Selected {
			out = append(out, f.Label)
		}
	}
	sort.Strings(out)
	return out
}

func checkFixCase(k FixCase, work string) error {
	before, out, err := runTests(filepath.Join(work, "head"), k.Head, k.Hidden)
	if err != nil {
		return err
	}
	for name, result := range before {
		wantFail := strings.HasPrefix(name, "TestAccept") && k.ExpectedResponse == "fixed"
		if (result == "fail") != wantFail {
			return fmt.Errorf("%s %ss on the seeded head:\n%s", name, result, out)
		}
	}
	responses := map[string]string{}
	for _, label := range k.selectedLabels() {
		responses[label] = k.ExpectedResponse
	}
	repaired := k.repaired()
	after, out, err := runTests(filepath.Join(work, "fix"), repaired, k.Hidden)
	if err != nil {
		return err
	}
	if s := ScoreFix(k, responses, repaired, false, after); !s.Fixed {
		return fmt.Errorf("reference repair does not score as fixed: %+v\n%s", s, out)
	}
	return nil
}
