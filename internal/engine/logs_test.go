package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogsPreserveCursorsAndRedactSplitSecrets(t *testing.T) {
	t.Setenv("GREENHOUSE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("WORKER_SECRET", "a-secret-across-chunks")
	writer := &logWriter{run: "run", step: "test"}
	writer.Write([]byte("before a-secret-"))
	view, err := readLogs("run", Request{Step: "test", Full: true})
	if err != nil || view.Bytes != 0 {
		t.Fatalf("partial output exposed: %v %v", view, err)
	}
	writer.Write([]byte("across-chunks\n"))
	writer.Write([]byte(strings.Repeat("plain output\n", 10000)))
	zero := int64(0)
	view, err = readLogs("run", Request{Step: "test", Offset: &zero})
	if err != nil || view.Offset != 0 || view.Next != 65536 || !strings.HasPrefix(view.Content, "before [REDACTED:WORKER_SECRET]") {
		t.Fatalf("explicit zero cursor: %v %v", view, err)
	}
	tail, err := readLogs("run", Request{Step: "test"})
	if err != nil || tail.Offset <= 0 || len(tail.Content) != 16384 {
		t.Fatalf("tail: %v %v", tail, err)
	}
	appendLog("run", "test", []byte(strings.Repeat("x", transcriptLimit)))
	full, err := readLogs("run", Request{Step: "test", Full: true})
	if err != nil || !full.Truncated || full.Bytes != transcriptLimit {
		t.Fatalf("bounded retention: %d %v %v", full.Bytes, full.Truncated, err)
	}
	home, _ := Home()
	info, err := os.Stat(filepath.Join(home, "logs", "run", "test.log"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private log: %v %v", info, err)
	}
}
