package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func TestLineOrderingHelper(t *testing.T) {
	if home := os.Getenv("GREENHOUSE_LINE_ORDER_TEST_HOME"); home != "" {
		if err := lineTerminal(home, "."); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLineDelayedIntermediateDaemonResponse(t *testing.T) {
	for _, responseFirst := range []bool{false, true} {
		name := "watch-before-response"
		if responseFirst {
			name = "response-before-watch"
		}
		t.Run(name, func(t *testing.T) {
			home, err := os.MkdirTemp("", "line-order-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(home) })
			listener, err := net.Listen("unix", filepath.Join(home, "socket"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }() // Best-effort test cleanup.
			ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
			defer stop()
			watch, response, requested, sent := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					go func() {
						defer func() { _ = conn.Close() }() // Best-effort test cleanup.
						var q engine.Request
						if json.NewDecoder(conn).Decode(&q) != nil {
							return
						}
						state := ready(tuiState("running", "cancel"))
						v := onStream(state, "daemon-A", 5, "token-A")
						switch q.Op {
						case "cancel":
							close(requested)
							select {
							case <-response:
							case <-ctx.Done():
								return
							}
							v = onStream(state, "daemon-B", 1, "token-B")
							v.Run.Status = "cancelling"
						case "watch":
							if q.Stream == "daemon-C" {
								<-ctx.Done()
								return
							}
							select {
							case <-watch:
							case <-ctx.Done():
								return
							}
							v = onStream(state, "daemon-C", 1, "token-C")
							v.Run.Status = "cancelled"
						}
						v.Version = q.Version
						v.Protocol = &engine.Protocol{Version: v.Version, Min: 1, Max: engine.MaxVersion}
						_ = json.NewEncoder(conn).Encode(v) // Best-effort HTTP test response.
						if q.Op == "watch" {
							close(sent)
						}
					}()
				}
			}()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLineOrderingHelper$")
			cmd.Env = append(os.Environ(), "GREENHOUSE_LINE_ORDER_TEST_HOME="+home, "TERM=dumb", "NO_COLOR=1", "GORACE=atexit_sleep_ms=0")
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { stop(); _ = cmd.Wait() }()
			screens := make(chan string)
			go func() {
				reader := bufio.NewReader(output)
				var page strings.Builder
				for {
					b, err := reader.ReadByte()
					if err != nil {
						return
					}
					page.WriteByte(b)
					if strings.HasSuffix(page.String(), "\n> ") {
						select {
						case screens <- page.String():
						case <-ctx.Done():
							return
						}
						page.Reset()
					}
				}
			}()
			next := func() string {
				t.Helper()
				select {
				case page := <-screens:
					return page
				case <-ctx.Done():
					t.Fatal("line client did not render a screen")
					return ""
				}
			}
			next()
			if _, err := io.WriteString(input, "x\n"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-requested:
			case <-ctx.Done():
				t.Fatal("cancel was not sent")
			}
			if !responseFirst {
				close(watch)
				select {
				case <-sent:
				case <-ctx.Done():
					t.Fatal("watch was not delivered")
				}
			}
			close(response)
			page := next()
			if strings.Contains(page, "cancelling") {
				t.Fatalf("delayed intermediate daemon response replaced follower state:\n%s", page)
			}
			if responseFirst {
				close(watch)
			}
			for !strings.Contains(page, "cancelled") {
				page = next()
			}
			if _, err := io.WriteString(input, "q\n"); err != nil {
				t.Fatal(err)
			}
			_ = input.Close() // Best-effort test cleanup.
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
