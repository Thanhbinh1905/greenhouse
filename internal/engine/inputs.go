package engine

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type InputCriterion struct {
	Pack           string `json:"pack"`
	Rule           string `json:"rule"`
	Digest         string `json:"digest"`
	Classification string `json:"classification"`
	Text           string `json:"text"`
}
type AgentInput struct {
	Version      int              `json:"version"`
	Role         string           `json:"role"`
	Candidate    string           `json:"candidate"`
	ConfigID     string           `json:"config_id"`
	ConfigDigest string           `json:"config_digest"`
	SnapshotID   string           `json:"snapshot_id"`
	Scope        string           `json:"scope"`
	Guidance     string           `json:"guidance"`
	Criteria     []InputCriterion `json:"criteria"`
	Repair       []string         `json:"repair_guidance,omitempty"`
}
type InputIdentity struct {
	Role       string `json:"role"`
	Candidate  string `json:"candidate"`
	SnapshotID string `json:"snapshot_id"`
	Digest     string `json:"digest"`
	Content    []byte `json:"content"`
}

func emitInput(w io.Writer, s RuleSnapshot, role string) (InputIdentity, error) {
	if role != "reviewer" && role != "fixer" {
		return InputIdentity{}, errors.New("unsupported role")
	}
	if !validText(s.Scope) {
		return InputIdentity{}, errors.New("missing rule scope")
	}
	if !rulesReady(s.Rules) {
		return InputIdentity{}, errors.New("required rules unresolved")
	}
	a := AgentInput{Version: 1, Role: role, Candidate: s.Candidate, ConfigID: s.ConfigID, ConfigDigest: s.ConfigDigest, SnapshotID: s.ID, Scope: s.Scope, Criteria: []InputCriterion{}}
	if role == "reviewer" {
		a.Guidance = "Use a fresh context independent of author history. Inspect the exact candidate read-only; report criteria violations, not repair procedures. No execution authority is granted."
	} else {
		a.Guidance = "Apply only explicitly authorized repairs within changed hunks under the executor's writer controls. These inputs do not authorize execution."
	}
	for _, r := range s.Rules {
		if r.Status != "resolved" {
			continue
		}
		p, err := parsePack(r.Bytes)
		if err != nil || r.Digest != runner.BytesDigest(r.Bytes) || !reflect.DeepEqual(p, r.Pack) {
			return InputIdentity{}, errors.New("resolved pack content mismatch")
		}
		for _, c := range r.Pack.Criteria {
			a.Criteria = append(a.Criteria, InputCriterion{r.Pack.ID, c.ID, r.Digest, c.Classification, c.Text})
			if role == "fixer" {
				a.Repair = append(a.Repair, r.Pack.ID+"/"+c.ID+": "+c.Repair)
			}
		}
	}
	b, e := json.Marshal(a)
	if e != nil {
		return InputIdentity{}, e
	}
	b = append(b, '\n')
	n, e := w.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	if e != nil {
		return InputIdentity{}, e
	}
	return InputIdentity{role, s.Candidate, s.ID, runner.BytesDigest(b), b}, nil
}
