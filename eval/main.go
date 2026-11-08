// Command eval measures Greenhouse independent review against a fixed corpus
// of small Git changes with known findings. It is a development tool and is
// not part of the greenhouse executable.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const usage = `usage:
  go run ./eval check [-corpus dir]
  go run ./eval judge -model provider/id [-pi path] [-base-url url] [-parallel n] [-corpus dir] run-dir...
  go run ./eval rescore [-corpus dir] run-dir...
  go run ./eval run -model provider/id -out dir [-label text] [-pi path] [-base-url url] [-judge-model provider/id] [-cases a,b] [-parallel n]`

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
		fmt.Printf("corpus v%d revision %d %s: %d cases, bug=%d policy=%d noise=%d clean=%d, %d high-severity findings\n", c.Version, c.Revision, c.Digest, len(c.Cases), count["bug"], count["policy"], count["noise"], count["clean"], high)
		return nil
	case "judge":
		o := judgeOptions{}
		fs.StringVar(&o.Model, "model", "", "exact provider/model of the judge")
		fs.StringVar(&o.Pi, "pi", "", "absolute Pi executable (default: pi on PATH)")
		fs.StringVar(&o.BaseURL, "base-url", "", "route the judge model's provider to this base URL")
		fs.IntVar(&o.Parallel, "parallel", 4, "concurrent judge calls")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if o.Model == "" || o.Parallel < 1 {
			return fmt.Errorf("-model and a positive -parallel are required\n%s", usage)
		}
		if o.Pi == "" {
			p, err := exec.LookPath("pi")
			if err != nil {
				return err
			}
			o.Pi = p
		}
		c, err := LoadCorpus(*corpus)
		if err != nil {
			return err
		}
		for _, dir := range fs.Args() {
			r, err := readReport(dir)
			if err != nil {
				return err
			}
			a, err := judgeReport(r, c, o)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if err := rescoreReport(r, c, a); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if err := writeAdjudications(dir, a); err != nil {
				return err
			}
			if err := writeReport(dir, r); err != nil {
				return err
			}
			fmt.Printf("%s: %s; bar met %t\n", dir, a.Judge.Note, r.Summary.BarMet)
		}
		return nil
	case "rescore":
		if err := fs.Parse(args); err != nil {
			return err
		}
		c, err := LoadCorpus(*corpus)
		if err != nil {
			return err
		}
		for _, dir := range fs.Args() {
			r, err := readReport(dir)
			if err != nil {
				return err
			}
			a, err := loadAdjudications(dir)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if err := rescoreReport(r, c, a); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if err := writeReport(dir, r); err != nil {
				return err
			}
			fmt.Printf("%s: bar met %t\n", dir, r.Summary.BarMet)
		}
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
		fs.StringVar(&o.JudgeModel, "judge-model", "", "judge model for adjudication records (default: -model)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		o.Corpus = *corpus
		if o.Model == "" || o.Out == "" || o.Parallel < 1 {
			return fmt.Errorf("-model, -out and a positive -parallel are required\n%s", usage)
		}
		if o.JudgeModel == "" {
			o.JudgeModel = o.Model
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

// readReport loads a saved report without its case scores, which callers
// recompute and which older reports stored in an earlier shape.
func readReport(dir string) (*Report, error) {
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		return nil, err
	}
	var raw struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	var r Report
	if err := json.Unmarshal(b, &struct {
		*Report
		Cases *[]map[string]json.RawMessage `json:"cases"`
	}{&r, new([]map[string]json.RawMessage)}); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	for _, c := range raw.Cases {
		delete(c, "score")
		cb, _ := json.Marshal(c)
		var rec CaseRecord
		if err := json.Unmarshal(cb, &rec); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		r.Cases = append(r.Cases, rec)
	}
	return &r, nil
}
