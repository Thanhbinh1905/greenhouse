package runner

import (
	"syscall"
	"testing"
	"time"
)

func TestFreezeChildTreeDoesNotWaitForStoppedState(t *testing.T) {
	const childPID = 1234
	calls := 0
	stops := 0
	started := time.Now()
	deadline := started.Add(100 * time.Millisecond)
	pids, complete := freezeChildTreeWith(deadline, func(int) (map[int]byte, error) {
		calls++
		return map[int]byte{childPID: 'D'}, nil
	}, func(pid int, signal syscall.Signal) error {
		if pid != childPID || signal != syscall.SIGSTOP {
			t.Fatalf("signal(%d, %v)", pid, signal)
		}
		stops++
		return nil
	})
	if !complete {
		t.Fatalf("freeze did not stabilize after repeated scans: calls=%d stops=%d pids=%v", calls, stops, pids)
	}
	if calls < 3 || stops < 2 {
		t.Fatalf("freeze did not scan and stop repeatedly: calls=%d stops=%d", calls, stops)
	}
	if len(pids) != 1 || pids[0] != childPID {
		t.Fatalf("frozen process set: %v", pids)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("freeze exceeded its deadline: %s", elapsed)
	}
}

func TestFreezeChildTreeReturnsDescendantsSeenBeforeDisappearing(t *testing.T) {
	const childPID = 2345
	scans := 0
	stopped := make(map[int]bool)
	pids, complete := freezeChildTreeWith(time.Now().Add(100*time.Millisecond), func(int) (map[int]byte, error) {
		scans++
		if scans == 1 {
			return map[int]byte{childPID: 'R'}, nil
		}
		return map[int]byte{}, nil
	}, func(pid int, signal syscall.Signal) error {
		if signal != syscall.SIGSTOP {
			t.Fatalf("signal(%d, %v)", pid, signal)
		}
		stopped[pid] = true
		return nil
	})
	if !complete {
		t.Fatalf("freeze did not stabilize: scans=%d pids=%v", scans, pids)
	}
	if len(pids) != 1 || pids[0] != childPID {
		t.Fatalf("freeze omitted previously observed descendant: %v", pids)
	}
	if !stopped[childPID] {
		t.Fatalf("descendant %d was not stopped", childPID)
	}
}

func TestFreezeChildTreeReturnsKnownProcessesAtDeadline(t *testing.T) {
	calls := 0
	deadline := time.Now().Add(25 * time.Millisecond)
	pids, complete := freezeChildTreeWith(deadline, func(int) (map[int]byte, error) {
		calls++
		return map[int]byte{1000 + calls: 'R'}, nil
	}, func(int, syscall.Signal) error { return nil })
	if complete {
		t.Fatalf("freeze unexpectedly completed with a new process on every scan: %v", pids)
	}
	if len(pids) < 2 {
		t.Fatalf("deadline result lost known process IDs: %v", pids)
	}
	if calls < 2 {
		t.Fatalf("freeze stopped scanning too early: %d scans", calls)
	}
}
