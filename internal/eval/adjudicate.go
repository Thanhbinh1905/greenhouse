package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const adjudicationSchema = "greenhouse.eval.adjudications.v1"

type Judge struct {
	Kind      string `json:"kind"`
	Model     string `json:"model,omitempty"`
	Backend   string `json:"backend,omitempty"`
	PiVersion string `json:"pi_version,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Note      string `json:"note,omitempty"`
}

// Record states whether one recorded finding identifies one expected defect.
// FindingDigest binds the record to the exact finding text it judged.
type Record struct {
	Case          string `json:"case"`
	Finding       int    `json:"finding"`
	FindingDigest string `json:"finding_digest"`
	Expected      string `json:"expected"`
	Verdict       string `json:"verdict"`
	Rationale     string `json:"rationale"`
}

type Adjudications struct {
	Schema  string   `json:"schema"`
	Judge   Judge    `json:"judge"`
	Records []Record `json:"records"`
}

func findingDigest(f Finding) string {
	b, _ := json.Marshal(f)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

var quotation = regexp.MustCompile(`"([^"]{12,})"`)

func cites(rationale string, f Finding) bool {
	text := f.Explanation + "\n" + f.Action
	for _, m := range quotation.FindAllStringSubmatch(rationale, -1) {
		if strings.Contains(text, m[1]) {
			return true
		}
	}
	return false
}

func loadAdjudications(dir string) (*Adjudications, error) {
	b, err := os.ReadFile(filepath.Join(dir, "adjudications.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Adjudications
	if err := strictDecode(b, &a); err != nil {
		return nil, fmt.Errorf("adjudications.json: %w", err)
	}
	if a.Schema != adjudicationSchema || (a.Judge.Kind != "model" && a.Judge.Kind != "manual") {
		return nil, errors.New("adjudications.json: unsupported schema or judge kind")
	}
	if a.Judge.Kind == "model" && (a.Judge.Model == "" || a.Judge.Prompt == "") {
		return nil, errors.New("adjudications.json: a model judge records its model and prompt")
	}
	return &a, nil
}

func writeAdjudications(dir string, a *Adjudications) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "adjudications.json"), append(b, '\n'), 0o644)
}
