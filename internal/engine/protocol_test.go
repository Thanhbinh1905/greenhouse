package engine

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtocolNegotiation(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	newer := axi(t, bin, repo, home, `{"version":3,"op":"attach"}`)
	protocol, _ := newer["protocol"].(map[string]any)
	if newer["outcome"] != "refused" || !strings.Contains(newer["error"].(string), "supports 1-2") || protocol["min"] != float64(1) || protocol["max"] != float64(2) {
		t.Fatalf("newer client must learn the supported range: %v", newer)
	}
	if newer["run_id"] != nil {
		t.Fatalf("refused version created or exposed a run: %v", newer)
	}
	for _, version := range []float64{1, 2} {
		request := `{"version":1,"op":"attach"}`
		if version == 2 {
			request = `{"version":2,"op":"attach"}`
		}
		v := axi(t, bin, repo, home, request)
		protocol, _ := v["protocol"].(map[string]any)
		if v["outcome"] != "ok" || v["version"] != version || protocol["version"] != version || protocol["max"] != float64(2) {
			t.Fatalf("version %v not negotiated: %v", version, v)
		}
	}
	old := axi(t, bin, repo, home, `{"version":1,"op":"watch"}`)
	if old["outcome"] != "refused" || !strings.Contains(old["error"].(string), "requires protocol version 2") {
		t.Fatalf("version-2 operation accepted at version 1: %v", old)
	}
}

func TestTerminalRefusesIncompatibleDaemon(t *testing.T) {
	t.Parallel()
	for _, daemon := range []struct {
		reply, want string
	}{
		{`{"version":1,"outcome":"refused","error":"unsupported protocol version"}`, "speaks only protocol 1"},
		{`{"version":3,"outcome":"refused","error":"unsupported protocol version 2; this daemon supports 3-4","protocol":{"version":3,"min":3,"max":4}}`, "running daemon supports 3-4"},
	} {
		bin, repo, home := fixture(t)
		os.Mkdir(home, 0700)
		listener, err := net.Listen("unix", filepath.Join(home, "socket"))
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			for {
				c, err := listener.Accept()
				if err != nil {
					return
				}
				json.NewDecoder(c).Decode(&map[string]any{})
				io.WriteString(c, daemon.reply+"\n")
				c.Close()
			}
		}()
		c := exec.Command(bin)
		c.Dir = repo
		c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
		c.Stdin = strings.NewReader("q\n")
		out, err := c.CombinedOutput()
		listener.Close()
		if err == nil || !strings.Contains(string(out), daemon.want) {
			t.Fatalf("terminal must refuse clearly, got %v:\n%s", err, out)
		}
	}
}
