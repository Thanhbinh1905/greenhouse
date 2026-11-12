package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
	"github.com/Thanhbinh1905/greenhouse/internal/terminal"
)

func Main() {
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			fmt.Print(`Usage:
  greenhouse              Open the interactive terminal screen.
                          Use the line client when stdin or stdout is not a
                          terminal, or TERM is unset or dumb.
  greenhouse axi          Read one JSON request from stdin; write one JSON
                          response to stdout. Invoke once per request.
  greenhouse daemon       Run the local state daemon.
  greenhouse [-h|--help]   Show this help (also: greenhouse axi --help).

Host review (protocol version 2):
  attach (or new) with host identity, apply settings, resolve-rules,
  prepare with intent (what the change should do) and target (an exact commit).
  Review uses the merge-base of target and the candidate as the diff base.
  review requires a host grant for each review round; watch follows progress.
  Read host_result for the verdict and check current before accepting it.

Complete host requests and response fields: docs/embedding.md
`)
			return
		}
	}
	h, e := engine.Home()
	if e == nil {
		path, _ := os.Getwd()
		switch {
		case len(os.Args) == 1:
			e = terminal.Run(h, path)
		case len(os.Args) == 2 && os.Args[1] == "daemon":
			e = engine.Daemon(h)
		case len(os.Args) == 2 && os.Args[1] == "axi":
			var q engine.Request
			d := json.NewDecoder(io.LimitReader(os.Stdin, 65536))
			d.DisallowUnknownFields()
			e = d.Decode(&q)
			if e == nil {
				q.Path = path
				q.Client = "axi"
				var v engine.Response
				v, e = engine.Send(h, q, true)
				if e == nil {
					e = json.NewEncoder(os.Stdout).Encode(v)
				}
			}
		default:
			e = errors.New("usage: greenhouse [axi|daemon]")
		}
	}
	if e != nil {
		if len(os.Args) > 1 && os.Args[1] == "axi" {
			json.NewEncoder(os.Stdout).Encode(engine.Response{Version: 1, Outcome: "error", Error: e.Error()})
		} else {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
}
