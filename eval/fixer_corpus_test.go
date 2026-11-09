package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFixCorpusReferenceRepairs(t *testing.T) {
	c, err := LoadFixCorpus(filepath.Join("testdata", "fixer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Cases) < 8 {
		t.Fatalf("corpus has %d cases, want at least 8", len(c.Cases))
	}
	for _, k := range c.Cases {
		t.Run(k.ID, func(t *testing.T) {
			t.Parallel()
			if err := checkFixCase(k, t.TempDir()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChangedLines(t *testing.T) {
	removed, inserted := changedLines("a\nb\nc\nd\n", "a\nB\nc\nx\nd\n")
	if !reflect.DeepEqual(removed, []int{2}) || !reflect.DeepEqual(inserted, []int{3}) {
		t.Fatalf("removed %v inserted %v", removed, inserted)
	}
}

func TestScopeViolations(t *testing.T) {
	head := map[string]string{"p/a.go": "1\n2\n3\n4\n5\n", "p/b.go": "x\n"}
	scope := []LineRange{{"p/a.go", 2, 3}}
	for _, c := range []struct {
		name     string
		repaired map[string]string
		want     []string
	}{
		{"inside", map[string]string{"p/a.go": "1\ntwo\n3\nnew\n4\n5\n", "p/b.go": "x\n"}, nil},
		{"outside line", map[string]string{"p/a.go": "1\n2\n3\n4\nfive\n", "p/b.go": "x\n"}, []string{"changed p/a.go:5"}},
		{"distant insert", map[string]string{"p/a.go": "1\n2\n3\n4\n5\n6\n", "p/b.go": "x\n"}, []string{"inserted after p/a.go:5"}},
		{"other file", map[string]string{"p/a.go": "1\n2\n3\n4\n5\n", "p/b.go": "y\n", "p/c.go": "z\n"}, []string{"changed p/b.go:1", "added p/c.go"}},
	} {
		if got := scopeViolations(head, c.repaired, scope); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScoreFixRequiresEveryCondition(t *testing.T) {
	k := FixCase{Head: map[string]string{"a.go": "1\n2\n"}, Scope: []LineRange{{"a.go", 1, 1}}, ExpectedResponse: "fixed", Findings: []SeededFinding{{Label: "f", Selected: true}, {Label: "decoy"}}}
	pass := TestOutcomes{"TestAcceptX": "pass", "TestRegressY": "pass", "TestVisible": "pass"}
	fixed := map[string]string{"a.go": "one\n2\n"}
	if s := ScoreFix(k, map[string]string{"f": "fixed"}, fixed, false, pass); !s.Fixed || !s.ResponseCorrect {
		t.Fatalf("clean repair: %+v", s)
	}
	for name, s := range map[string]FixScore{
		"disputed":     ScoreFix(k, map[string]string{"f": "disputed"}, fixed, false, pass),
		"no response":  ScoreFix(k, nil, fixed, false, pass),
		"accept fails": ScoreFix(k, map[string]string{"f": "fixed"}, fixed, false, TestOutcomes{"TestAcceptX": "fail", "TestRegressY": "pass"}),
		"regression":   ScoreFix(k, map[string]string{"f": "fixed"}, fixed, false, TestOutcomes{"TestAcceptX": "pass", "TestVisible": "fail"}),
		"out of scope": ScoreFix(k, map[string]string{"f": "fixed"}, map[string]string{"a.go": "one\ntwo\n"}, false, pass),
		"refused":      ScoreFix(k, map[string]string{"f": "fixed"}, fixed, true, pass),
	} {
		if s.Fixed {
			t.Errorf("%s scored as fixed: %+v", name, s)
		}
	}
	sum := SummarizeFix([]FixScore{{Fixed: true}, {Fixed: true}, {Fixed: true}, {Fixed: false}})
	if !sum.BarMet || *sum.FixedRate != 0.75 {
		t.Fatalf("75%% with no regression or scope violation meets the bar: %+v", sum)
	}
	if SummarizeFix([]FixScore{{Fixed: true}, {Fixed: true}, {Fixed: true}, {Fixed: true, Regressions: []string{"TestRegressY"}}}).BarMet {
		t.Fatal("a regression on any case misses the bar")
	}
}
