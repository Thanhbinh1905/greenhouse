package main

import "testing"

func TestUnresolvedOptionalSelectionDoesNotBlock(t *testing.T) {
	r := &Run{Candidate: "head", Configs: []Snapshot{{ID: "config", Settings: Settings{Rules: []string{"?missing"}}}}}
	rules, ready := effectiveRules(r)
	if !ready || len(rules) != 1 || rules[0].Status != "configured" {
		t.Fatal("optional configured selection blocked eligibility", rules, ready)
	}
}
