package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBlockedCatalogPreparationCanBeCancelled(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout"} {
		for _, role := range []string{"reviewer", "fixer"} {
			t.Run(role+"/"+mode, func(t *testing.T) {
				var bin, repo, home string
				var v map[string]any
				if role == "fixer" {
					bin, repo, home, v = repairFixture(t)
				} else {
					bin, repo, home, v = prepareFixture(t)
				}
				root := filepath.Dir(home)
				exe := "fake-pi"
				if role == "fixer" {
					exe = "fake-fixer"
				}
				script := `#!/usr/bin/python3
import os,sys,time,subprocess
if '--list-models' in sys.argv:
 root=os.path.dirname(__file__)
 if os.path.basename(__file__) == 'fake-pi' and not os.path.exists(root+'/catalog-preflight'):
  open(root+'/catalog-preflight','w').close()
  print('fixture model')
  sys.exit(0)
 child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)'])
 with open(os.path.dirname(__file__)+'/catalog-started','w') as f: f.write(str(child.pid))
 time.sleep(60)
sys.exit(1)
`
				writeExecutable(t, filepath.Join(root, exe), script)
				op := "review"
				var selected []string
				if role == "fixer" {
					op, selected = "repair", selectedID(t, v)
				}
				if got := repairRequest(t, bin, repo, home, v, op, "start-blocked", selected); got["outcome"] != "ok" {
					t.Fatal(got)
				}
				deadline := time.Now().Add(10 * time.Second)
				for {
					if b, _ := os.ReadFile(filepath.Join(root, "catalog-started")); len(b) > 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("catalog did not start")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if mode == "cancel" {
					current := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
					if got := repairRequest(t, bin, repo, home, current, "cancel", "cancel-catalog", nil); got["outcome"] != "ok" {
						t.Fatalf("cancel during %s provider preparation refused: %v", role, got["error"])
					}
				}
				var done map[string]any
				deadline = time.Now().Add(8 * time.Second)
				for time.Now().Before(deadline) {
					done = runOf(t, axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`))
					if done["status"] != "running" && done["status"] != "writing" && done["status"] != "cancelling" {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				want := "cancelled"
				if mode == "timeout" {
					want = "inconclusive"
					if role == "fixer" {
						want = "unconfigured"
					}
				}
				if done["status"] != want {
					t.Fatalf("%s catalog cancellation did not settle: %v", role, done["status"])
				}
				part := "review"
				if role == "fixer" {
					part = "repair"
				}
				execution := done[part].(map[string]any)["id"].(string)
				records, _ := filepath.Glob(filepath.Join(home, "executors", execution+".catalog.*.json"))
				if len(records) != 1 {
					t.Fatalf("catalog must have one tracked executor: %v", records)
				}
				var rec executorRecord
				if err := json.Unmarshal(mustRead(t, records[0]), &rec); err != nil || !rec.Reaped || !rec.Stopped {
					t.Fatalf("catalog descendants not stopped and reaped: %+v %v", rec, err)
				}
				if strings.Contains(string(mustRead(t, records[0])), "secret") {
					t.Fatal("catalog record leaked credentials")
				}
			})
		}
	}
}
