package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var expectedID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

var builtinRules = map[string][]string{
	"unslop":      {"unslop/plain-prose"},
	"no-comments": {"no-comments/necessary-comments"},
}

type Corpus struct {
	Version  int    `json:"version"`
	Revision int    `json:"revision"`
	Digest   string `json:"digest"`
	Cases    []Case `json:"-"`
}

type Case struct {
	Split       string            `json:"split"`
	Stratum     string            `json:"stratum"`
	Tags        []string          `json:"tags"`
	Source      Source            `json:"source"`
	Allowed     []Allowed         `json:"allowed,omitempty"`
	Version     int               `json:"version"`
	ID          string            `json:"id"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Intent      string            `json:"intent"`
	Rules       []string          `json:"rules"`
	Expected    []Expected        `json:"expected"`
	Noise       []Noise           `json:"noise"`
	Base        map[string]string `json:"-"`
	Head        map[string]string `json:"-"`
}

type Source struct {
	Kind       string   `json:"kind"`
	Repo       string   `json:"repo,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	License    string   `json:"license,omitempty"`
	URL        string   `json:"url,omitempty"`
	Files      []string `json:"files,omitempty"`
	Derivation string   `json:"derivation"`
}

type Allowed struct {
	File   string `json:"file"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Origin string `json:"origin"`
	Rule   string `json:"rule"`
	Note   string `json:"note"`
}

var strata = map[string]bool{
	"reversed-bug-fix": true, "injected-defect": true, "clean-real-change": true,
	"security-sensitive": true, "refactor-only": true, "large-change": true,
	"tiny-change": true, "cross-file": true, "judgment": true,
}

var publicLicenses = map[string]bool{"MIT": true, "Apache-2.0": true, "BSD-2-Clause": true, "BSD-3-Clause": true, "ISC": true}
var commitID = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validateMetadata(k Case) error {
	if k.Split != "tune" && k.Split != "holdout" {
		return errors.New("split must be tune or holdout")
	}
	if !strata[k.Stratum] {
		return fmt.Errorf("invalid stratum %q", k.Stratum)
	}
	s := k.Source
	if strings.TrimSpace(s.Derivation) == "" {
		return errors.New("source derivation required")
	}
	switch s.Kind {
	case "synthetic":
	case "local", "public":
		if s.Repo == "" || !commitID.MatchString(s.Commit) || len(s.Files) == 0 {
			return errors.New("real source requires repo, exact commit and files")
		}
		if s.Kind == "public" && (!publicLicenses[s.License] || !strings.HasPrefix(s.URL, "https://")) {
			return errors.New("public source requires allowed license and https url")
		}
	default:
		return fmt.Errorf("unknown source kind %q", s.Kind)
	}
	for _, name := range s.Files {
		if name == "" || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || name == ".." || strings.HasPrefix(name, "../") {
			return errors.New("source files must be repository-relative paths")
		}
		if _, base := k.Base[name]; !base {
			if _, head := k.Head[name]; !head {
				return fmt.Errorf("source file %s absent from case trees", name)
			}
		}
	}
	for _, tag := range k.Tags {
		if strings.TrimSpace(tag) == "" {
			return errors.New("empty tag")
		}
	}
	if len(k.Allowed) > 0 && k.Category != "judgment" {
		return errors.New("allowed ranges require judgment category")
	}
	for _, a := range k.Allowed {
		if err := checkRange(k.Head, a.File, a.Start, a.End); err != nil {
			return err
		}
		if a.Note == "" || (a.Origin != "correctness" && a.Origin != "policy") || (a.Origin == "correctness" && a.Rule != "") {
			return errors.New("allowed range requires note and valid origin/rule")
		}
		if a.Origin == "policy" {
			selected := false
			for _, r := range k.Rules {
				for _, id := range builtinRules[r] {
					if id == a.Rule {
						selected = true
					}
				}
			}
			if !selected {
				return errors.New("allowed rule not selected")
			}
		}
	}
	return nil
}

type Expected struct {
	ID         string   `json:"id"`
	File       string   `json:"file"`
	Start      int      `json:"start"`
	End        int      `json:"end"`
	Origin     string   `json:"origin"`
	Rule       string   `json:"rule"`
	Severity   string   `json:"severity"`
	Priorities []string `json:"priorities"`
	Defect     string   `json:"defect"`
}

type Noise struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Note  string `json:"note"`
}

func strictDecode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func LoadCorpus(root string) (*Corpus, error) {
	b, err := os.ReadFile(filepath.Join(root, "corpus.json"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Version  int      `json:"version"`
		Revision int      `json:"revision"`
		Cases    []string `json:"cases"`
	}
	if err := strictDecode(b, &manifest); err != nil {
		return nil, fmt.Errorf("corpus.json: %w", err)
	}
	if manifest.Version != 1 {
		return nil, errors.New("corpus.json: unsupported version")
	}
	listed := map[string]bool{}
	for _, id := range manifest.Cases {
		if !expectedID.MatchString(id) || listed[id] {
			return nil, fmt.Errorf("invalid or duplicate case ID %q", id)
		}
		listed[id] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && !listed[e.Name()] {
			return nil, fmt.Errorf("case directory %s not listed in corpus.json", e.Name())
		}
	}
	c := &Corpus{Version: manifest.Version, Revision: manifest.Revision}
	for _, id := range manifest.Cases {
		k, err := loadCase(filepath.Join(root, id), id)
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", id, err)
		}
		c.Cases = append(c.Cases, *k)
	}
	c.Digest, err = treeDigest(root)
	return c, err
}

func loadCase(dir, id string) (*Case, error) {
	b, err := os.ReadFile(filepath.Join(dir, "case.json"))
	if err != nil {
		return nil, err
	}
	var k Case
	if err := strictDecode(b, &k); err != nil {
		return nil, err
	}
	if k.Version != 1 {
		return nil, errors.New("unsupported case version")
	}
	if k.ID != id {
		return nil, fmt.Errorf("id %q does not match directory", k.ID)
	}
	if strings.TrimSpace(k.Intent) == "" {
		return nil, errors.New("intent required")
	}
	if k.Base, err = readTree(filepath.Join(dir, "base")); err != nil {
		return nil, err
	}
	if k.Head, err = readTree(filepath.Join(dir, "head")); err != nil {
		return nil, err
	}
	changed := false
	for name, body := range k.Head {
		if k.Base[name] != body {
			changed = true
		}
	}
	if len(k.Base) != len(k.Head) {
		changed = true
	}
	if !changed {
		return nil, errors.New("base and head have no change")
	}
	selected := map[string]bool{}
	for _, r := range k.Rules {
		ids, ok := builtinRules[r]
		if !ok {
			return nil, fmt.Errorf("unsupported rule selection %q", r)
		}
		for _, id := range ids {
			selected[id] = true
		}
	}
	switch k.Category {
	case "judgment":
		if len(k.Allowed) == 0 {
			return nil, errors.New("judgment case needs allowed ranges")
		}
	case "clean", "noise":
		if len(k.Expected) != 0 {
			return nil, fmt.Errorf("%s case must not expect findings", k.Category)
		}
	case "bug", "policy":
		if len(k.Expected) == 0 {
			return nil, fmt.Errorf("%s case needs expected findings", k.Category)
		}
	default:
		return nil, fmt.Errorf("unknown category %q", k.Category)
	}
	ids := map[string]bool{}
	for _, e := range k.Expected {
		if err := checkRange(k.Head, e.File, e.Start, e.End); err != nil {
			return nil, err
		}
		if e.Origin == "policy" && e.Rule == "" || e.Origin == "correctness" && e.Rule != "" || e.Origin != "policy" && e.Origin != "correctness" {
			return nil, fmt.Errorf("invalid origin/rule pair %q/%q", e.Origin, e.Rule)
		}
		if e.Origin == "policy" && !selected[e.Rule] {
			return nil, fmt.Errorf("rule %s not selected by the case", e.Rule)
		}
		if e.Severity != "high" && e.Severity != "low" {
			return nil, fmt.Errorf("severity %q must be high or low", e.Severity)
		}
		if len(e.Priorities) == 0 {
			return nil, errors.New("priority list required")
		}
		for _, p := range e.Priorities {
			if !validPriority(p) {
				return nil, fmt.Errorf("invalid priority %q", p)
			}
		}
		if strings.TrimSpace(e.Defect) == "" {
			return nil, errors.New("defect description required")
		}
		if !expectedID.MatchString(e.ID) || ids[e.ID] {
			return nil, fmt.Errorf("expected ID %q must be unique and match %s", e.ID, expectedID)
		}
		ids[e.ID] = true
	}
	for _, n := range k.Noise {
		if err := checkRange(k.Head, n.File, n.Start, n.End); err != nil {
			return nil, err
		}
	}
	if err := validateMetadata(k); err != nil {
		return nil, err
	}
	return &k, nil
}

func validPriority(p string) bool { return p == "P0" || p == "P1" || p == "P2" || p == "P3" }

func checkRange(tree map[string]string, file string, start, end int) error {
	body, ok := tree[file]
	if !ok {
		return fmt.Errorf("%s not in head tree", file)
	}
	lines := strings.Count(body, "\n")
	if !strings.HasSuffix(body, "\n") {
		lines++
	}
	if start < 1 || end < start || end > lines {
		return fmt.Errorf("range %s:%d-%d outside the file's %d lines", file, start, end, lines)
	}
	return nil
}

func readTree(dir string) (map[string]string, error) {
	tree := map[string]string{}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return tree, nil
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		tree[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return tree, err
}

func treeDigest(root string) (string, error) {
	var names []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.New()
	for _, p := range names {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
