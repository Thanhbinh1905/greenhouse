package engine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNativeCatalogHasBoundedLifetime(t *testing.T) {
	bin, repo, state, v := prepareFixture(t)
	root := filepath.Dir(state)
	script := `#!/usr/bin/python3
import subprocess,sys,time,os
if '--list-models' in sys.argv:
 child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'])
 open(os.path.dirname(__file__)+'/catalog-child','w').write(str(child.pid))
 time.sleep(60)
`
	writeExecutable(t, filepath.Join(root, "fake-pi"), script)
	started := time.Now()
	result := axi(t, bin, repo, state, `{"version":1,"op":"models"}`)
	if result["outcome"] != "refused" || time.Since(started) > 8*time.Second {
		t.Fatalf("catalog did not stop within deadline: %v", result)
	}
	child, err := os.ReadFile(filepath.Join(root, "catalog-child"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(child)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("catalog child %d survived timeout: %v", pid, err)
	}
	current := axi(t, bin, repo, state, `{"version":1,"op":"inspect"}`)
	if current["run_id"] != v["run_id"] || runOf(t, current)["review_used"] != float64(0) {
		t.Fatal("catalog changed review or usage")
	}
}
