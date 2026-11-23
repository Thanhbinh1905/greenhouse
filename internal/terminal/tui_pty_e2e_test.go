//go:build linux

package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type ptyScreen struct {
	master *os.File
	mu     sync.Mutex
	out    bytes.Buffer
}

func openPTY(t *testing.T, cmd *exec.Cmd, cols, rows uint16) *ptyScreen {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		t.Fatal(e)
	}
	var number uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), unix.TIOCGPTN, uintptr(unsafe.Pointer(&number))); e != 0 {
		t.Fatal(e)
	}
	slave, err := os.OpenFile("/dev/pts/"+itoa(number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	size := unix.Winsize{Row: rows, Col: cols}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &size); err != nil {
		t.Fatal(err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()
	screen := &ptyScreen{master: master}
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				screen.mu.Lock()
				screen.out.Write(buf[:n])
				screen.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = master.Close()
	})
	return screen
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}

func (s *ptyScreen) mark() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.Len()
}

func (s *ptyScreen) waitFor(t *testing.T, since int, want string) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		text := s.out.String()[since:]
		s.mu.Unlock()
		if strings.Contains(text, want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Fatalf("screen never showed %q; recent output:\n%q", want, s.out.String()[max(0, s.out.Len()-1500):])
}

func (s *ptyScreen) send(t *testing.T, text string) {
	t.Helper()
	if _, err := s.master.WriteString(text); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestTUIEditedIntentReachesTheNextReview(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	stateHome, err := os.MkdirTemp("", "gh-pty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{repo, home, binDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "init", "-q", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repo, "sample.txt", "base\n")
	gitTest(t, repo, "add", "sample.txt")
	gitTest(t, repo, "commit", "-qm", "base")
	gitTest(t, repo, "checkout", "-qb", "feature")
	writeTestFile(t, repo, "sample.txt", "changed\n")
	gitTest(t, repo, "commit", "-qam", "Improve sample handling")
	agent := strings.Replace(fakeReviewAgent, "'explanation':'Sample issue found'", "'explanation':'Reviewed with intent: '+b['Intent']", 1)
	writeTestFile(t, binDir, "pi", agent)
	if err := os.Chmod(filepath.Join(binDir, "pi"), 0700); err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "greenhouse")
	if output, err := exec.Command("go", "build", "-o", binary, "../../cmd/greenhouse").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repo
	cmd.Env = testEnvironment("PATH="+binDir+":/usr/bin", "HOME="+home, "GREENHOUSE_HOME="+stateHome, "TERM=xterm-256color")
	screen := openPTY(t, cmd, 100, 30)
	t.Cleanup(func() {
		stop := exec.Command(binary, "stop")
		stop.Dir, stop.Env = repo, cmd.Env
		_ = stop.Run()
	})
	screen.waitFor(t, 0, "Reviewed with intent: Improve sample handling")

	at := screen.mark()
	screen.send(t, "e")
	screen.waitFor(t, at, "Improve sample handling")
	screen.send(t, "\x15Edited by the person")
	screen.send(t, "\r")
	screen.send(t, "\r")
	screen.send(t, "\r")
	screen.waitFor(t, at, "Saved for Run")
	screen.waitFor(t, at, "Intent   Edited by the person")
	screen.send(t, "a")
	screen.waitFor(t, at, "Start a new run")
	for i := 0; i < 12; i++ {
		at = screen.mark()
		screen.send(t, "j")
		time.Sleep(100 * time.Millisecond)
		screen.mu.Lock()
		tail := screen.out.String()[at:]
		screen.mu.Unlock()
		if strings.Contains(tail, "▸ Start a new run") || strings.Contains(tail, "▸Start a new run") {
			break
		}
	}
	screen.send(t, "\r")
	screen.send(t, "new\r")
	screen.waitFor(t, at, "New run ready")
	screen.send(t, "\r")
	screen.waitFor(t, at, "Reviewed with intent: Edited by the person")
}
