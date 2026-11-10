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
  go run ./eval judge -model provider/id [-pi path] [-parallel n] [-corpus dir] [-split tune|holdout|all] run-dir...
  go run ./eval rescore [-corpus dir] [-split tune|holdout|all] run-dir...
  go run ./eval run -model provider/id -out dir [-label text] [-pi path] [-judge-model provider/id] [-cases a,b] [-split tune|holdout|all] [-parallel n]
  go run ./eval fix-check [-corpus dir]
  go run ./eval fix-run -out dir [-model provider/id] [-label text] [-pi path] [-price in,out,cache-read,cache-write] [-cases a,b] [-parallel n]`

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
	defaultCorpus := "eval/testdata/corpus"
	if strings.HasPrefix(sub, "fix-") {
		defaultCorpus = "eval/testdata/fixer"
	}
	split := "all"
	if sub == "run" || sub == "judge" || sub == "rescore" {
		fs.StringVar(&split, "split", "all", "case split: tune, holdout or all")
	}
	corpus := fs.String("corpus", defaultCorpus, "corpus directory")
	switch sub {
	case "fix-check":
		if err := fs.Parse(args); err != nil {
			return err
		}
		c, err := LoadFixCorpus(*corpus)
		if err != nil {
			return err
		}
		work, err := os.MkdirTemp("", "gfc")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		for _, k := range c.Cases {
			if err := checkFixCase(k, filepath.Join(work, k.ID)); err != nil {
				return fmt.Errorf("%s: %w", k.ID, err)
			}
		}
		fmt.Printf("fixer corpus v%d revision %d %s: %d cases; seeded heads fail and reference repairs pass the hidden tests\n", c.Version, c.Revision, c.Digest, len(c.Cases))
		return nil
	case "fix-run":
		o := fixOptions{Settle: 15 * time.Minute}
		fs.StringVar(&o.Model, "model", "openai/gpt-5.6-luna", "exact provider/model of the evaluation fixer")
		fs.StringVar(&o.Out, "out", "", "new directory for the report and captures")
		fs.StringVar(&o.Label, "label", "", "short description recorded in the report")
		fs.StringVar(&o.Pi, "pi", "", "absolute Pi executable (default: pi on PATH)")
		fs.StringVar(&o.BaseURL, "base-url", "", "retired; use native agent provider settings")
		price := fs.String("price", "0.2,1.2,0.02,0.25", "USD per million input, output, cache-read and cache-write tokens, used when Pi reports no cost")
		cases := fs.String("cases", "", "comma-separated case IDs (default: all)")
		fs.IntVar(&o.Parallel, "parallel", 1, "cases repaired concurrently")
		if err := fs.Parse(args); err != nil {
			return err
		}
		o.Corpus = *corpus
		if o.Out == "" || o.Parallel < 1 {
			return fmt.Errorf("-out and a positive -parallel are required\n%s", usage)
		}
		if _, err := fmt.Sscanf(*price, "%g,%g,%g,%g", &o.Price.Input, &o.Price.Output, &o.Price.CacheRead, &o.Price.CacheWrite); err != nil {
			return fmt.Errorf("-price: %w", err)
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
		r, err := runFixLive(o)
		if err != nil {
			return err
		}
		fmt.Printf("bar met: %t; report: %s/report.md\n", r.Summary.BarMet, o.Out)
		return nil
	case "check":
		if err := fs.Parse(args); err != nil {
			return err
		}
		c, err := LoadCorpus(*corpus)
		if err != nil {
			return err
		}
		coverage, err := CheckCoverage(c)
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
		fmt.Printf("corpus v%d revision %d %s: %d cases, bug=%d policy=%d noise=%d clean=%d judgment=%d, %d high-severity findings\n", c.Version, c.Revision, c.Digest, len(c.Cases), count["bug"], count["policy"], count["noise"], count["clean"], count["judgment"], high)
		fmt.Print(coverageText(coverage))
		return nil
	case "judge":
		o := judgeOptions{}
		fs.StringVar(&o.Model, "model", "", "exact provider/model of the judge")
		fs.StringVar(&o.Pi, "pi", "", "absolute Pi executable (default: pi on PATH)")
		fs.StringVar(&o.BaseURL, "base-url", "", "retired; use native Pi provider settings")
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
			if err := filterReport(r, c, split); err != nil {
				return err
			}
			a, err := judgeReport(r, c, o)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			if err := rescoreReport(r, c, a); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			target := dir
			if split != "all" {
				target = filepath.Join(dir, split)
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			if err := writeAdjudications(target, a); err != nil {
				return err
			}
			if err := writeReport(target, r); err != nil {
				return err
			}
			fmt.Printf("%s: %s; bar met %t\n", target, a.Judge.Note, r.Summary.BarMet)
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
			if err := filterReport(r, c, split); err != nil {
				return err
			}
			if split != "all" {
				a = filterAdjudications(a, r)
			}
			if err := rescoreReport(r, c, a); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			target := dir
			if split != "all" {
				target = filepath.Join(dir, split)
			}
			if a != nil && split != "all" {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
				if err := writeAdjudications(target, a); err != nil {
					return err
				}
			}
			if err := writeReport(target, r); err != nil {
				return err
			}
			fmt.Printf("%s: bar met %t\n", target, r.Summary.BarMet)
		}
		return nil
	case "run":
		o := runOptions{Settle: 12 * time.Minute}
		fs.StringVar(&o.Model, "model", "", "exact native agent model; provider/model for Pi")
		fs.StringVar(&o.Out, "out", "", "new directory for the report and captures")
		fs.StringVar(&o.Label, "label", "", "short description recorded in the report")
		fs.StringVar(&o.Pi, "pi", "", "absolute Pi executable (default: pi on PATH)")
		fs.StringVar(&o.BaseURL, "base-url", "", "retired; use native agent provider settings")
		fs.StringVar(&o.Backend, "backend", "pi", "reviewer backend: pi or claude")
		fs.StringVar(&o.Agent, "agent", "", "absolute executable of a non-Pi backend (default: the backend name on PATH)")
		cases := fs.String("cases", "", "comma-separated case IDs (default: all)")
		fs.IntVar(&o.Parallel, "parallel", 1, "cases reviewed concurrently")
		fs.StringVar(&o.JudgeModel, "judge-model", "", "judge model for adjudication records (default: -model)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		o.Corpus = *corpus
		o.Split = split
		if _, err := normalizedSplit(split); err != nil {
			return err
		}
		if o.Model == "" || o.Out == "" || o.Parallel < 1 {
			return fmt.Errorf("-model, -out and a positive -parallel are required\n%s", usage)
		}
		if o.Backend != "pi" && o.Backend != "claude" {
			return fmt.Errorf("-backend must be pi or claude\n%s", usage)
		}
		if o.Backend == "claude" && o.Agent == "" {
			p, err := exec.LookPath("claude")
			if err != nil {
				return err
			}
			o.Agent = p
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
