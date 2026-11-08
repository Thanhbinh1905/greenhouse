// Command eval measures Greenhouse independent review against a fixed corpus
// of small Git changes with known findings. It is a development tool and is
// not part of the greenhouse executable.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const usage = `usage:
  go run ./eval check [-corpus dir]
  go run ./eval run -model provider/id -out dir [-label text] [-pi path] [-base-url url] [-cases a,b] [-parallel n]`

func main() {
	if cfg := os.Getenv(shimEnv); cfg != "" {
		os.Exit(runShim(cfg))
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := dispatch(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(sub string, args []string) error {
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	corpus := fs.String("corpus", "eval/testdata/corpus", "corpus directory")
	switch sub {
	case "check":
		if err := fs.Parse(args); err != nil {
			return err
		}
		c, err := LoadCorpus(*corpus)
		if err != nil {
			return err
		}
		count := map[string]int{}
		high := 0
		for _, k := range c.Cases {
			count[k.Category]++
			for _, e := range k.Expected {
				if e.Severity == "high" {
					high++
				}
			}
		}
		fmt.Printf("corpus v%d %s: %d cases, bug=%d policy=%d noise=%d clean=%d, %d high-severity findings\n", c.Version, c.Digest, len(c.Cases), count["bug"], count["policy"], count["noise"], count["clean"], high)
		return nil
	case "run":
		o := runOptions{Settle: 12 * time.Minute}
		fs.StringVar(&o.Model, "model", "", "exact provider/model for reviewer.json")
		fs.StringVar(&o.Out, "out", "", "new directory for the report and captures")
		fs.StringVar(&o.Label, "label", "", "short description recorded in the report")
		fs.StringVar(&o.Pi, "pi", "", "absolute Pi executable (default: pi on PATH)")
		fs.StringVar(&o.BaseURL, "base-url", "", "route the model's provider to this base URL inside the reviewer's private agent directory")
		cases := fs.String("cases", "", "comma-separated case IDs (default: all)")
		fs.IntVar(&o.Parallel, "parallel", 1, "cases reviewed concurrently")
		if err := fs.Parse(args); err != nil {
			return err
		}
		o.Corpus = *corpus
		if o.Model == "" || o.Out == "" || o.Parallel < 1 {
			return fmt.Errorf("-model, -out and a positive -parallel are required\n%s", usage)
		}
		if *cases != "" {
			o.Cases = strings.Split(*cases, ",")
		}
		if o.Pi == "" {
			p, err := exec.LookPath("pi")
			if err != nil {
				return err
			}
			o.Pi = p
		}
		r, err := runLive(o)
		if err != nil {
			return err
		}
		fmt.Printf("bar met: %t; report: %s/report.md\n", r.Summary.BarMet, o.Out)
		return nil
	}
	return fmt.Errorf("unknown subcommand %q\n%s", sub, usage)
}
