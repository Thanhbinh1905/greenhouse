package main

import "fmt"

func normalizedSplit(split string) (string, error) {
	if split == "" {
		split = "all"
	}
	if split != "all" && split != "tune" && split != "holdout" {
		return "", fmt.Errorf("split must be tune, holdout or all")
	}
	return split, nil
}

func selectCases(c *Corpus, split string, ids []string) ([]Case, error) {
	split, err := normalizedSplit(split)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var selected []Case
	for _, k := range c.Cases {
		if split != "all" && k.Split != split {
			continue
		}
		if len(ids) > 0 && !wanted[k.ID] {
			continue
		}
		selected = append(selected, k)
		delete(wanted, k.ID)
	}
	if len(wanted) > 0 {
		return nil, fmt.Errorf("cases absent from split %s: %v", split, wanted)
	}
	return selected, nil
}

func filterReport(r *Report, c *Corpus, split string) error {
	split, err := normalizedSplit(split)
	if err != nil {
		return err
	}
	byID := map[string]Case{}
	for _, k := range c.Cases {
		byID[k.ID] = k
	}
	var selected []CaseRecord
	for _, rec := range r.Cases {
		k, ok := byID[rec.ID]
		if !ok {
			return fmt.Errorf("case %s not in corpus", rec.ID)
		}
		if split == "all" || k.Split == split {
			selected = append(selected, rec)
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("report has no cases in split %s", split)
	}
	r.Cases = selected
	r.Config.Cases = nil
	for _, rec := range selected {
		r.Config.Cases = append(r.Config.Cases, rec.ID)
	}
	r.Config.Split = split
	return nil
}

func filterAdjudications(a *Adjudications, r *Report) *Adjudications {
	if a == nil {
		return nil
	}
	selected := map[string]bool{}
	for _, rec := range r.Cases {
		selected[rec.ID] = true
	}
	b := *a
	b.Records = nil
	for _, rec := range a.Records {
		if selected[rec.Case] {
			b.Records = append(b.Records, rec)
		}
	}
	return &b
}
