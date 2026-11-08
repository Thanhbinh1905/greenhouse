package main

import (
	"testing"
)

func TestRuleResolutionCLI(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	resolved := mutate(t, bin, repo, home, v, "resolve-rules", "resolve-1", nil)
	if resolved["outcome"] != "ok" {
		t.Fatalf("explicit resolution refused: %v", resolved["error"])
	}
	rules, ok := runOf(t, resolved)["rule_snapshots"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("missing rule snapshot: %v", resolved)
	}
	if resolved["rule_scope"] != policyScope {
		t.Fatal("AXI omitted common scope and exceptions")
	}
	if resolved["rules_ready"] != true {
		t.Fatal(resolved)
	}
}
