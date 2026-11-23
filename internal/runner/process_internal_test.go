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
	ops := pidfdOperations{
		open: func(pid int) (int, error) { return pid + 10000, nil },
		signal: func(fd int, signal syscall.Signal) error {
			if fd != childPID+10000 {
				t.Fatalf("signal fd %d, want %d", fd, childPID+10000)
			}
			if signal == syscall.SIGSTOP {
				stops++
			}
			return nil
		},
		close: func(int) error { return nil },
	}
	handles, complete := freezeChildTreeWith(deadline, func(int) (map[int]byte, error) {
		calls++
		return map[int]byte{childPID: 'D'}, nil
	}, ops)
	if !complete {
		t.Fatalf("freeze did not stabilize after repeated scans: calls=%d stops=%d handles=%v", calls, stops, handles)
	}
	if calls < 3 || stops < 2 {
		t.Fatalf("freeze did not scan and stop repeatedly: calls=%d stops=%d", calls, stops)
	}
	if len(handles) != 1 || handles[0].pid != childPID {
		t.Fatalf("frozen process set: %v", handles)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("freeze exceeded its deadline: %s", elapsed)
	}
}

func TestFreezeChildTreeKillsDescendantThatDisappearsFromLatestScan(t *testing.T) {
	const (
		childPID = 2345
		childFD  = 12345
	)
	scans := 0
	var signals []struct {
		fd  int
		sig syscall.Signal
	}
	var closed []int
	ops := pidfdOperations{
		open: func(pid int) (int, error) {
			if pid != childPID {
				t.Fatalf("opened pidfd for unexpected PID %d", pid)
			}
			return childFD, nil
		},
		signal: func(fd int, signal syscall.Signal) error {
			signals = append(signals, struct {
				fd  int
				sig syscall.Signal
			}{fd, signal})
			return nil
		},
		close: func(fd int) error {
			closed = append(closed, fd)
			return nil
		},
	}
	handles, complete := freezeChildTreeWith(time.Now().Add(100*time.Millisecond), func(int) (map[int]byte, error) {
		scans++
		if scans == 1 {
			return map[int]byte{childPID: 'R'}, nil
		}
		return map[int]byte{}, nil
	}, ops)
	if !complete {
		t.Fatalf("freeze did not stabilize: scans=%d handles=%v", scans, handles)
	}
	if err := killProcessHandles(handles, ops.signal, ops.close); err != nil {
		t.Fatal(err)
	}
	stopped, killed := false, false
	for _, call := range signals {
		stopped = stopped || call.fd == childFD && call.sig == syscall.SIGSTOP
		killed = killed || call.fd == childFD && call.sig == syscall.SIGKILL
	}
	if !stopped || !killed || len(closed) != 1 || closed[0] != childFD {
		t.Fatalf("disappeared descendant cleanup: signals=%v closed=%v", signals, closed)
	}
}

func TestFreezeChildTreeReopensPidfdAfterPIDReuse(t *testing.T) {
	const childPID = 3456
	scans := 0
	nextFD := 100
	var signals []struct {
		fd  int
		sig syscall.Signal
	}
	var closed []int
	ops := pidfdOperations{
		open: func(int) (int, error) {
			nextFD++
			return nextFD, nil
		},
		signal: func(fd int, signal syscall.Signal) error {
			signals = append(signals, struct {
				fd  int
				sig syscall.Signal
			}{fd, signal})
			if fd == 101 && signal == 0 {
				return syscall.ESRCH
			}
			return nil
		},
		close: func(fd int) error {
			closed = append(closed, fd)
			return nil
		},
	}
	handles, complete := freezeChildTreeWith(time.Now().Add(100*time.Millisecond), func(int) (map[int]byte, error) {
		scans++
		if scans < 4 {
			return map[int]byte{childPID: 'R'}, nil
		}
		return map[int]byte{}, nil
	}, ops)
	if !complete || len(handles) != 1 || handles[0].pidfd != 102 {
		t.Fatalf("freeze did not replace the stale pidfd: complete=%v handles=%v", complete, handles)
	}
	if err := killProcessHandles(handles, ops.signal, ops.close); err != nil {
		t.Fatal(err)
	}
	killedStale, killedCurrent := false, false
	for _, call := range signals {
		killedStale = killedStale || call.fd == 101 && call.sig == syscall.SIGKILL
		killedCurrent = killedCurrent || call.fd == 102 && call.sig == syscall.SIGKILL
	}
	if killedStale || !killedCurrent || len(closed) != 2 || closed[0] != 101 || closed[1] != 102 {
		t.Fatalf("PID reuse cleanup: signals=%v closed=%v", signals, closed)
	}
}

func TestFreezeChildTreeReturnsKnownProcessesAtDeadline(t *testing.T) {
	calls := 0
	deadline := time.Now().Add(25 * time.Millisecond)
	ops := pidfdOperations{
		open:   func(pid int) (int, error) { return pid + 10000, nil },
		signal: func(int, syscall.Signal) error { return nil },
		close:  func(int) error { return nil },
	}
	handles, complete := freezeChildTreeWith(deadline, func(int) (map[int]byte, error) {
		calls++
		return map[int]byte{1000 + calls: 'R'}, nil
	}, ops)
	if complete {
		t.Fatalf("freeze unexpectedly completed with a new process on every scan: %v", handles)
	}
	if len(handles) < 2 {
		t.Fatalf("deadline result lost known process IDs: %v", handles)
	}
	if calls < 2 {
		t.Fatalf("freeze stopped scanning too early: %d scans", calls)
	}
}
