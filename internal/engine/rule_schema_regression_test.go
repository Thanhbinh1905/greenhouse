package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaAliasesRejectedCLI(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	path := filepath.Join(t.TempDir(), "aliased.json")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"file:" + path}}
	v = mutate(t, bin, repo, home, v, "apply", "select-alias", settings)
	for _, tc := range []struct{ name, raw string }{
		{"version", strings.Replace(builtins["no-comments"], `"version":1`, `"version":2,"Version":1`, 1)},
		{"classification", strings.Replace(builtins["no-comments"], `"classification":"required"`, `"classification":"required","Classification":"advisory"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := os.WriteFile(path, []byte(tc.raw), 0600); e != nil {
				t.Fatal(e)
			}
			v = mutate(t, bin, repo, home, v, "resolve-rules", "resolve-alias-"+tc.name, nil)
			if v["outcome"] != "ok" {
				t.Fatal(v)
			}
			rule := v["rules"].([]any)[0].(map[string]any)
			if v["rules_ready"] != false || rule["status"] != "failed" {
				t.Fatalf("schema alias bypass: ready=%v status=%v pack=%v", v["rules_ready"], rule["status"], rule["pack"])
			}
			if !strings.Contains(rule["error"].(string), "field") {
				t.Fatal("missing actionable schema error", rule)
			}
		})
	}
}

func TestSchemaFieldNamesAreExact(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"version", "id", "criteria", "classification", "text", "repair_guidance"} {
		for _, alias := range []string{strings.ToUpper(field), strings.ToUpper(field[:1]) + field[1:]} {
			for _, duplicate := range []bool{false, true} {
				raw := builtins["no-comments"]
				key := `"` + field + `":`
				if duplicate {
					values := map[string]string{"version": "1", "id": `"shadow"`, "criteria": "[]", "classification": `"advisory"`, "text": `"shadow"`, "repair_guidance": `"shadow"`}
					raw = strings.Replace(raw, key, `"`+alias+`":`+values[field]+`,`+key, 1)
				} else {
					raw = strings.Replace(raw, key, `"`+alias+`":`, 1)
				}
				if _, e := parsePack([]byte(raw)); e == nil {
					t.Errorf("accepted alias %q (duplicate=%t)", alias, duplicate)
				}
			}
		}
	}
	for _, alias := range []string{"ID", "Id"} {
		for _, key := range []string{`"` + alias + `":"necessary-comments"`, `"id":"necessary-comments","` + alias + `":"shadow"`} {
			raw := strings.Replace(builtins["no-comments"], `"id":"necessary-comments"`, key, 1)
			if _, err := parsePack([]byte(raw)); err == nil {
				t.Errorf("accepted criterion ID alias %q", alias)
			}
		}
	}
	for _, raw := range []string{
		strings.Replace(builtins["no-comments"], `"version"`, `"verſion"`, 1),
		strings.Replace(builtins["no-comments"], `"classification"`, `"claſſification"`, 1),
	} {
		if _, e := parsePack([]byte(raw)); e == nil {
			t.Error("accepted Unicode case-fold alias")
		}
	}
	if _, e := parsePack([]byte(builtins["no-comments"])); e != nil {
		t.Fatal("canonical schema refused", e)
	}
}
