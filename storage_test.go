package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDamagedStoreRefusedBeforeBinding(t *testing.T) {
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	stopDaemon(t, home)
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var s Store
	json.Unmarshal(b, &s)
	s.Runs[v["run_id"].(string)].Configs = nil
	persist(home, &s)
	before, _ := os.ReadFile(filepath.Join(home, "state.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "daemon")
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	if b, e := cmd.CombinedOutput(); e == nil || ctx.Err() != nil {
		t.Fatalf("damaged store accepted: %s", b)
	}
	after, _ := os.ReadFile(filepath.Join(home, "state.json"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("damaged evidence rewritten")
	}
}
func TestRetainedUsageAndLimitEdits(t *testing.T) {
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	stopDaemon(t, home)
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var s Store
	json.Unmarshal(b, &s)
	r := s.Runs[v["run_id"].(string)]
	r.ReviewUsed = 2
	r.RepairUsed = 1
	persist(home, &s)
	v = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	settings := Settings{"auto", "one-pass", 2, []string{"unslop"}}
	if mutate(t, bin, repo, home, v, "apply", "reduce", settings)["outcome"] != "refused" {
		t.Fatal("consumed usage erased")
	}
	settings.Limit = 8
	applied := mutate(t, bin, repo, home, v, "apply", "increase", settings)
	if applied["outcome"] != "ok" {
		t.Fatal(applied)
	}
	rr := runOf(t, applied)
	if rr["review_used"] != float64(2) || rr["repair_used"] != float64(1) {
		t.Fatal("usage reset")
	}
	events := rr["events"].([]any)
	last := events[len(events)-1].(map[string]any)
	if last["used"] != float64(3) || last["before"] != float64(4) || last["after"] != float64(8) {
		t.Fatal("missing cap provenance")
	}
}
