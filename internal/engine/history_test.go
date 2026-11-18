package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLargeLegacyHistoryDoesNotEnterHotState(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	stopDaemon(t, home)
	data, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	source := s.Runs[v["run_id"].(string)]
	for i := 0; i < 40; i++ {
		r := *source
		r.ID = fmt.Sprintf("history-%03d", i)
		r.SelectedDiff = strings.Repeat("x", 1024*1024)
		s.Runs[r.ID] = &r
		s.Receipts[r.ID] = Receipt{Digest: r.ID, Result: Response{Run: &r}}
	}
	data, err = json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 35*1024*1024 {
		t.Fatalf("fixture too small: %d", len(data))
	}
	if err := os.WriteFile(filepath.Join(home, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(filepath.Dir(home), "release-history-review")
	t.Cleanup(func() { os.WriteFile(release, nil, 0600) })
	installFake(t, home, "while not os.path.exists("+strconv.Quote(release)+"): time.sleep(0.02)")
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	v = mutate(t, bin, repo, home, v, "review", "large-history-review", nil)
	for i := 0; i < 3; i++ {
		start := time.Now()
		got := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		if got["outcome"] != "ok" || runOf(t, got)["status"] != "running" {
			t.Fatal(got)
		}
		elapsed := time.Since(start)
		t.Logf("inspect during executor: %s", elapsed)
		if elapsed > 2*time.Second {
			t.Fatalf("inspect took %s", elapsed)
		}
	}
	hot, err := os.Stat(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy bytes: %d; hot bytes: %d", len(data), hot.Size())
	if hot.Size() > 2*1024*1024 {
		t.Fatalf("hot state still contains settled history: %d bytes", hot.Size())
	}
	old := axi(t, bin, repo, home, `{"version":1,"op":"inspect","run_id":"history-039"}`)
	if old["outcome"] != "ok" || runOf(t, old)["selected_diff"] != strings.Repeat("x", 1024*1024) {
		t.Fatal("historical inspect lost retained payload")
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitReview(t, bin, repo, home)
	stopDaemon(t, home)
	restarted := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if restarted["run_id"] != source.ID {
		t.Fatal("slot changed during migration")
	}
}

func TestHistorySaveCostIsBounded(t *testing.T) {
	home := t.TempDir()
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	var plateauBytes int
	for i := 0; i < historyLimit*3; i++ {
		id := fmt.Sprintf("run-%04d", i)
		s.Runs[id] = &Run{ID: id, Status: "cancelled", SelectedDiff: strings.Repeat("x", 1024)}
		s.Receipts[id] = Receipt{Digest: id, Result: Response{Run: s.Runs[id]}}
		writes, bytes := 0, 0
		if err := persistWithWriter(home, s, func(path string, data []byte) error {
			writes++
			bytes += len(data)
			return writeFileDurably(path, data)
		}); err != nil {
			t.Fatal(err)
		}
		if writes != 3 || bytes > 64*1024 {
			t.Fatalf("save %d cost: %d writes, %d bytes", i, writes, bytes)
		}
		if i == historyLimit-1 {
			plateauBytes = bytes
		}
		if i >= historyLimit && bytes != plateauBytes {
			t.Fatalf("save bytes grew after retention filled: first=%d later=%d", plateauBytes, bytes)
		}
		if i == 0 || i == historyLimit-1 || i == historyLimit*3-1 {
			t.Logf("after %d runs: %d writes, %d bytes", i+1, writes, bytes)
		}
		if len(s.Runs) != 0 || len(s.Receipts) != 0 {
			t.Fatal("history retained in hot maps")
		}
	}
	for _, kind := range []string{"runs", "receipts"} {
		files, err := os.ReadDir(filepath.Join(home, "history", kind))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != historyLimit {
			t.Fatalf("%s retained %d records", kind, len(files))
		}
	}
	before, err := os.Stat(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Runs["oversized"] = &Run{ID: "oversized", Status: "cancelled", SelectedDiff: strings.Repeat("y", historyBytes+1)}
	if err := persist(home, s); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() {
		t.Fatal("save bytes grow with history")
	}
	old, err := historicalRun(s, "run-0000")
	if err != nil || old != nil {
		t.Fatalf("expired history not pruned: %v", err)
	}
	for _, kind := range []string{"runs", "receipts"} {
		files, err := os.ReadDir(filepath.Join(home, "history", kind))
		if err != nil {
			t.Fatal(err)
		}
		var size int64
		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				t.Fatal(err)
			}
			size += info.Size()
		}
		if size > historyBytes {
			t.Fatalf("%s exceeds byte retention: %d", kind, size)
		}
	}
}

func TestMigrationPreservesActiveCustodyAndTrust(t *testing.T) {
	home := t.TempDir()
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	makeRun := func(id, status string) *Run {
		return &Run{ID: id, Input: "input", Token: "token", Candidate: "head", Status: status, Configs: []Snapshot{{ID: "config", Settings: s.Defaults}}}
	}
	s.Runs["slot"] = makeRun("slot", "unconfigured")
	s.Slots["owner"] = "slot"
	s.Runs["active"] = makeRun("active", "running")
	s.Runs["custody"] = makeRun("custody", "needs-decision")
	s.Runs["custody"].Pipeline = &Pipeline{Worktree: "private-candidate", Status: "needs-decision"}
	s.Trust["repo"] = Trust{Repository: "repo", Digest: "trusted", Actor: "user"}
	s.Runs["settled"] = makeRun("settled", "cancelled")
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	migrated, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := persist(home, migrated); err != nil {
		t.Fatal(err)
	}
	reloaded, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Slots["owner"] != "slot" || reloaded.Runs["slot"] == nil || reloaded.Runs["active"].Recovery == nil || reloaded.Runs["custody"].Pipeline.Worktree != "private-candidate" || reloaded.Trust["repo"].Digest != "trusted" {
		t.Fatal("migration lost slot, active recovery, custody or trust")
	}
	old, err := historicalRun(reloaded, "settled")
	if err != nil || old == nil {
		t.Fatalf("settled run lost: %v", err)
	}
}

func TestUncommittedReceiptIsNotReplayed(t *testing.T) {
	home := t.TempDir()
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{Digest: "request", Result: Response{Outcome: "ok"}}
	if err := archive(home, "receipts", "action", receipt); err != nil {
		t.Fatal(err)
	}
	if _, found, err := historicalReceipt(s, "action"); err != nil || found {
		t.Fatalf("uncommitted receipt visible: %v", err)
	}
	s.Receipts["action"] = receipt
	if err := persist(home, s); err != nil {
		t.Fatal(err)
	}
	reloaded, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	if got, found, err := historicalReceipt(reloaded, "action"); err != nil || !found || got.Digest != receipt.Digest {
		t.Fatalf("committed receipt missing: %v", err)
	}
}

func TestFailedStateCommitPreservesCommittedHistory(t *testing.T) {
	home := t.TempDir()
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < historyLimit; i++ {
		id := fmt.Sprintf("committed-%03d", i)
		r := &Run{ID: id, Input: "input", Token: "token", Candidate: "head", Status: "cancelled", Configs: []Snapshot{{ID: "config", Settings: s.Defaults}}}
		s.Runs[id] = r
		s.Receipts[id] = Receipt{Digest: id, Result: Response{Run: r}}
	}
	if err := persist(home, s); err != nil {
		t.Fatal(err)
	}
	oldest := "committed-000"
	for _, kind := range []string{"runs", "receipts"} {
		if err := os.Chtimes(historyPath(home, kind, oldest), time.Unix(1, 0), time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Runs["new"] = &Run{ID: "new", Status: "cancelled"}
	s.Receipts["new"] = Receipt{Digest: "new"}
	if err := os.Chmod(home, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(home, 0700) })
	if err := persist(home, s); err == nil {
		t.Fatal("state commit unexpectedly succeeded")
	}
	after, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed save changed committed state")
	}
	restored, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := historicalReceipt(restored, oldest); err != nil || !found {
		t.Fatalf("failed commit pruned a committed receipt: %v", err)
	}
	if r, err := historicalRun(restored, oldest); err != nil || r == nil {
		t.Fatalf("failed commit pruned a committed run: %v", err)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	restored.Runs["new"] = &Run{ID: "new", Status: "cancelled"}
	restored.Receipts["new"] = Receipt{Digest: "new"}
	if err := persistWithWriter(home, restored, func(path string, data []byte) error {
		if filepath.Base(path) == "state.json" {
			for _, kind := range []string{"runs", "receipts"} {
				if _, err := os.Stat(historyPath(home, kind, oldest)); err != nil {
					t.Fatalf("pruned %s before durable state commit: %v", kind, err)
				}
			}
		}
		return writeFileDurably(path, data)
	}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"runs", "receipts"} {
		if _, err := os.Stat(historyPath(home, kind, oldest)); !os.IsNotExist(err) {
			t.Fatalf("%s not collected after durable commit: %v", kind, err)
		}
	}
}
