package eval

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

type Coverage struct {
	Total       int            `json:"total"`
	Sources     map[string]int `json:"sources"`
	Splits      map[string]int `json:"splits"`
	Strata      map[string]int `json:"strata"`
	Languages   map[string]int `json:"languages"`
	RealHoldout int            `json:"real_holdout"`
	Large       int            `json:"large"`
	Tiny        int            `json:"tiny"`
}

func editLineCount(a, b string) int {
	x, y := splitLines(a), splitLines(b)
	for len(x) > 0 && len(y) > 0 && x[0] == y[0] {
		x, y = x[1:], y[1:]
	}
	for len(x) > 0 && len(y) > 0 && x[len(x)-1] == y[len(y)-1] {
		x, y = x[:len(x)-1], y[:len(y)-1]
	}
	if len(x) < len(y) {
		x, y = y, x
	}
	row := make([]int, len(y)+1)
	for _, left := range x {
		prev := 0
		for j, right := range y {
			next := row[j+1]
			if left == right {
				row[j+1] = prev + 1
			} else {
				row[j+1] = max(row[j+1], row[j])
			}
			prev = next
		}
	}
	return len(x) + len(y) - 2*row[len(y)]
}

func caseSize(k Case) int {
	paths := map[string]bool{}
	for p := range k.Base {
		paths[p] = true
	}
	for p := range k.Head {
		paths[p] = true
	}
	n := 0
	for p := range paths {
		if k.Base[p] != k.Head[p] {
			n += editLineCount(k.Base[p], k.Head[p])
		}
	}
	return n
}

func caseLanguages(k Case) map[string]bool {
	found := map[string]bool{}
	extensions := map[string]string{".go": "Go", ".py": "Python", ".ts": "TypeScript", ".tsx": "TypeScript", ".rs": "Rust", ".java": "Java"}
	for _, tree := range []map[string]string{k.Base, k.Head} {
		for p := range tree {
			if k.Base[p] == k.Head[p] {
				continue
			}
			if lang := extensions[filepath.Ext(p)]; lang != "" {
				found[lang] = true
			}
		}
	}
	return found
}

func CheckCoverage(c *Corpus) (Coverage, error) {
	v := Coverage{Total: len(c.Cases), Sources: map[string]int{}, Splits: map[string]int{}, Strata: map[string]int{}, Languages: map[string]int{}}
	seen := map[string]bool{}
	realStrata := map[string]int{}
	commitSplits := map[string]string{}
	for _, k := range c.Cases {
		if seen[k.ID] {
			return v, fmt.Errorf("duplicate case %s", k.ID)
		}
		seen[k.ID] = true
		if err := validateMetadata(k); err != nil {
			return v, fmt.Errorf("%s: %w", k.ID, err)
		}

		if k.Source.Kind != "synthetic" {
			key := k.Source.Repo + ":" + k.Source.Commit
			if split, ok := commitSplits[key]; ok && split != k.Split {
				return v, fmt.Errorf("source commit %s crosses splits", key)
			}
			commitSplits[key] = k.Split
			realStrata[k.Stratum]++
		}
		v.Sources[k.Source.Kind]++
		v.Splits[k.Split]++
		traits := map[string]bool{k.Stratum: true}
		for _, tag := range k.Tags {
			traits[tag] = true
		}
		for tag := range traits {
			if strata[tag] {
				v.Strata[tag]++
			}
		}
		for lang := range caseLanguages(k) {
			v.Languages[lang]++
		}
		if k.Split == "holdout" && k.Source.Kind != "synthetic" {
			v.RealHoldout++
		}
		size := caseSize(k)
		if size > 1000 {
			v.Large++
		}
		if size > 0 && size < 10 {
			v.Tiny++
		}
		if traits["large-change"] && size <= 1000 {
			return v, fmt.Errorf("%s: large-change has only %d changed lines", k.ID, size)
		}
		if traits["tiny-change"] && (size == 0 || size >= 10) {
			return v, fmt.Errorf("%s: tiny-change has %d changed lines", k.ID, size)
		}
		if traits["cross-file"] {
			unchanged := false
			for p, b := range k.Head {
				if old, ok := k.Base[p]; ok && old == b {
					unchanged = true
				}
			}
			if !unchanged || len(k.Expected) == 0 {
				return v, fmt.Errorf("%s: cross-file needs an unchanged caller and expected defect", k.ID)
			}
			for _, e := range k.Expected {
				if k.Base[e.File] == k.Head[e.File] {
					return v, fmt.Errorf("%s: cross-file cause is outside diff", k.ID)
				}
			}
		}
		if traits["judgment"] && (k.Category != "judgment" || len(k.Allowed) == 0) {
			return v, fmt.Errorf("%s: judgment requires allowed findings", k.ID)
		}
	}
	checks := []struct {
		name     string
		got, min int
	}{
		{"total", v.Total, 150}, {"local", v.Sources["local"], 60}, {"public", v.Sources["public"], 60}, {"synthetic", v.Sources["synthetic"], 30},
		{"holdout", v.Splits["holdout"], 50}, {"real holdout", v.RealHoldout, 30},
		{"reversed-bug-fix", realStrata["reversed-bug-fix"], 30}, {"injected-defect", realStrata["injected-defect"], 20},
		{"clean-real-change", realStrata["clean-real-change"], 30}, {"security-sensitive", v.Strata["security-sensitive"], 10},
		{"refactor-only", realStrata["refactor-only"], 10}, {"cross-file", v.Strata["cross-file"], 8}, {"judgment", v.Strata["judgment"], 8},
		{"large changes", v.Large, 5}, {"tiny changes", v.Tiny, 5},
	}
	for _, ch := range checks {
		if ch.got < ch.min {
			return v, fmt.Errorf("coverage %s=%d; need at least %d", ch.name, ch.got, ch.min)
		}
	}
	for _, lang := range []string{"Go", "Python", "TypeScript"} {
		if v.Languages[lang] == 0 {
			return v, fmt.Errorf("coverage missing %s", lang)
		}
	}
	if v.Languages["Rust"]+v.Languages["Java"] == 0 {
		return v, fmt.Errorf("coverage requires Rust or Java")
	}
	if v.Languages["Go"]*100 > v.Total*60 {
		return v, fmt.Errorf("go exceeds 60 percent: %d/%d", v.Languages["Go"], v.Total)
	}
	return v, nil
}

func coverageText(v Coverage) string {
	var out strings.Builder
	fmt.Fprintf(&out, "coverage: total=%d real_holdout=%d large=%d tiny=%d\n", v.Total, v.RealHoldout, v.Large, v.Tiny)
	for _, group := range []struct {
		name   string
		counts map[string]int
	}{{"source", v.Sources}, {"split", v.Splits}, {"stratum", v.Strata}, {"language", v.Languages}} {
		keys := make([]string, 0, len(group.counts))
		for k := range group.counts {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		fmt.Fprintf(&out, "%s:", group.name)
		for _, k := range keys {
			fmt.Fprintf(&out, " %s=%d", k, group.counts[k])
		}
		out.WriteByte('\n')
	}
	return out.String()
}
