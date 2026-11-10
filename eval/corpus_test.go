package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCase(t *testing.T, root, id, spec string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, id, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, id, "case.json"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "corpus.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const bugSpec = `{"version":1,"id":"bug","category":"bug","intent":"Add LastN","split":"tune","stratum":"injected-defect","source":{"kind":"synthetic","derivation":"Constructed boundary test."},"rules":["unslop","no-comments"],
"expected":[{"id":"read-past-end","file":"a.go","start":2,"end":3,"origin":"correctness","rule":"","severity":"high","priorities":["P0","P1"],"defect":"reads past the end"}],
"noise":[{"file":"a.go","start":1,"end":1,"note":"package clause"}]}`

func TestLoadCorpusReadsManifestCasesAndTrees(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `{"version":1,"revision":3,"cases":["bug"]}`)
	writeCase(t, root, "bug", bugSpec, map[string]string{"base/a.go": "package a\n", "head/a.go": "package a\nx\ny\n"})
	c, err := LoadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 || c.Revision != 3 || len(c.Cases) != 1 || len(c.Digest) != 64 {
		t.Fatalf("corpus = %+v", c)
	}
	k := c.Cases[0]
	if k.ID != "bug" || k.Base["a.go"] != "package a\n" || k.Head["a.go"] != "package a\nx\ny\n" || k.Expected[0].Severity != "high" || k.Noise[0].Note != "package clause" {
		t.Fatalf("case = %+v", k)
	}
	before := c.Digest
	os.WriteFile(filepath.Join(root, "bug", "head", "a.go"), []byte("package a\nx\nz\n"), 0o644)
	c, err = LoadCorpus(root)
	if err != nil || c.Digest == before {
		t.Fatalf("digest must cover tree bytes: %v %s", err, c.Digest)
	}
}

func TestLoadCorpusRejectsInvalidGroundTruth(t *testing.T) {
	tests := []struct {
		name, spec, want string
		files            map[string]string
	}{
		{"unknown field", strings.Replace(bugSpec, `"intent"`, `"extra":1,"intent"`, 1), "unknown field", nil},
		{"id mismatch", strings.Replace(bugSpec, `"id":"bug"`, `"id":"other"`, 1), "does not match directory", nil},
		{"file missing from head", strings.Replace(bugSpec, `"file":"a.go","start":2`, `"file":"b.go","start":2`, 1), "not in head", nil},
		{"range beyond file", strings.Replace(bugSpec, `"end":3`, `"end":9`, 1), "outside", nil},
		{"policy without rule", strings.Replace(bugSpec, `"origin":"correctness","rule":""`, `"origin":"policy","rule":""`, 1), "rule", nil},
		{"rule not selected", strings.Replace(bugSpec, `"origin":"correctness","rule":""`, `"origin":"policy","rule":"team/x"`, 1), "not selected", nil},
		{"bad priority", strings.Replace(bugSpec, `["P0","P1"]`, `["P5"]`, 1), "priority", nil},
		{"clean with findings", strings.Replace(bugSpec, `"category":"bug"`, `"category":"clean"`, 1), "clean", nil},
		{"bug without findings", `{"version":1,"id":"bug","category":"bug","intent":"x","rules":[],"expected":[],"noise":[]}`, "expected", nil},
		{"missing expected ID", strings.Replace(bugSpec, `"id":"read-past-end",`, ``, 1), "expected ID", nil},
		{"invalid expected ID", strings.Replace(bugSpec, `"id":"read-past-end"`, `"id":"Read Past"`, 1), "expected ID", nil},
		{"no change", bugSpec, "no change", map[string]string{"base/a.go": "package a\nx\ny\n", "head/a.go": "package a\nx\ny\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, `{"version":1,"cases":["bug"]}`)
			files := tc.files
			if files == nil {
				files = map[string]string{"base/a.go": "package a\n", "head/a.go": "package a\nx\ny\n"}
			}
			writeCase(t, root, "bug", tc.spec, files)
			_, err := LoadCorpus(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadCorpusRejectsUnlistedAndMissingCases(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, `{"version":1,"cases":["bug","gone"]}`)
	writeCase(t, root, "bug", bugSpec, map[string]string{"base/a.go": "package a\n", "head/a.go": "package a\nx\ny\n"})
	if _, err := LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("missing case error = %v", err)
	}
	writeManifest(t, root, `{"version":1,"cases":[]}`)
	if _, err := LoadCorpus(root); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("unlisted case error = %v", err)
	}
}

func TestCommittedCorpusMeetsCoverage(t *testing.T) {
	c, err := LoadCorpus(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	high := 0
	rules := map[string]bool{}
	for _, k := range c.Cases {
		count[k.Category]++
		for _, e := range k.Expected {
			if e.Severity == "high" {
				high++
			}
			rules[e.Rule] = true
		}
	}
	if len(c.Cases) < 12 || count["clean"] < 3 || count["noise"] < 1 || count["policy"] < 2 || high < 5 {
		t.Fatalf("coverage = %v high=%d cases=%d", count, high, len(c.Cases))
	}
	for _, r := range []string{"unslop/plain-prose", "no-comments/necessary-comments"} {
		if !rules[r] {
			t.Fatalf("no case covers %s", r)
		}
	}
}

func TestCaseMetadataValidation(t *testing.T) {
	for _, tc := range []struct{ field, value, want string }{
		{"split", "", "split"}, {"split", "test", "split"},
		{"stratum", "unknown", "stratum"},
	} {
		t.Run(tc.field+tc.value, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, `{"version":1,"revision":5,"cases":["bug"]}`)
			var spec map[string]any
			if err := json.Unmarshal([]byte(bugSpec), &spec); err != nil {
				t.Fatal(err)
			}
			spec["split"] = "tune"
			spec["stratum"] = "injected-defect"
			spec["source"] = map[string]string{"kind": "synthetic", "derivation": "Constructed boundary test."}
			spec[tc.field] = tc.value
			b, _ := json.Marshal(spec)
			writeCase(t, root, "bug", string(b), map[string]string{"base/a.go": "package a\n", "head/a.go": "package a\nx\ny\n"})
			_, err := LoadCorpus(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %s", err, tc.want)
			}
		})
	}
}

func TestSplitSelection(t *testing.T) {
	c := &Corpus{Cases: []Case{{ID: "a", Split: "tune"}, {ID: "b", Split: "holdout"}, {ID: "c", Split: "holdout"}}}
	got, err := selectCases(c, "holdout", nil)
	if err != nil || len(got) != 2 || got[0].ID != "b" {
		t.Fatalf("cases=%v err=%v", got, err)
	}
	if _, err := selectCases(c, "private", nil); err == nil {
		t.Fatal("invalid split accepted")
	}
	if _, err := selectCases(c, "holdout", []string{"a"}); err == nil {
		t.Fatal("case outside split accepted")
	}
	r := &Report{Cases: []CaseRecord{{ID: "a"}, {ID: "b"}, {ID: "c"}}, Config: RunConfig{Cases: []string{"a", "b", "c"}}}
	if err := filterReport(r, c, "holdout"); err != nil {
		t.Fatal(err)
	}
	if r.Config.Split != "holdout" || len(r.Cases) != 2 || len(r.Config.Cases) != 2 {
		t.Fatalf("report=%+v", r)
	}
}

func TestPublicProvenanceValidation(t *testing.T) {
	for _, field := range []string{"commit", "license", "repo", "url", "files"} {
		t.Run(field, func(t *testing.T) {
			root := t.TempDir()
			writeManifest(t, root, `{"version":1,"revision":5,"cases":["bug"]}`)
			var spec map[string]any
			json.Unmarshal([]byte(bugSpec), &spec)
			source := map[string]any{"kind": "public", "repo": "https://example.org/project", "url": "https://example.org/commit/" + strings.Repeat("a", 40), "commit": strings.Repeat("a", 40), "license": "MIT", "files": []string{"a.go"}, "derivation": "Reversed known bounds fix."}
			delete(source, field)
			spec["source"] = source
			b, _ := json.Marshal(spec)
			writeCase(t, root, "bug", string(b), map[string]string{"base/a.go": "package a\n", "head/a.go": "package a\nx\ny\n"})
			if _, err := LoadCorpus(root); err == nil {
				t.Fatalf("public source without %s accepted", field)
			}
		})
	}
}
