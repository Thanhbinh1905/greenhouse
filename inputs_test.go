package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type failedInputWriter struct{}

func (failedInputWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture transport failure")
}

func TestFixtureInstructionDeliveryNotCompliance(t *testing.T) {
	t.Parallel()
	s := RuleSnapshot{Scope: policyScope, ID: "fixture", Candidate: "head", ConfigID: "config", ConfigDigest: "config-digest", Rules: []RuleEvidence{}}
	for _, name := range []string{"unslop", "no-comments"} {
		p, e := parsePack([]byte(builtins[name]))
		if e != nil {
			t.Fatal(e)
		}
		s.Rules = append(s.Rules, RuleEvidence{Status: "resolved", Pack: p, Bytes: []byte(builtins[name]), Digest: bytesDigest([]byte(builtins[name]))})
	}
	var reviewer, fixer bytes.Buffer
	a, e := emitInput(&reviewer, s, "reviewer")
	if e != nil {
		t.Fatal(e)
	}
	b, e := emitInput(&fixer, s, "fixer")
	if e != nil {
		t.Fatal(e)
	}
	var ar, br AgentInput
	if json.Unmarshal(reviewer.Bytes(), &ar) != nil || json.Unmarshal(fixer.Bytes(), &br) != nil {
		t.Fatal("invalid emitted input")
	}
	if !reflect.DeepEqual(ar.Criteria, br.Criteria) || ar.Scope != policyScope || len(ar.Repair) != 0 || len(br.Repair) != 2 {
		t.Fatal(ar, br)
	}
	if identity, err := emitInput(failedInputWriter{}, s, "reviewer"); err == nil || identity.Digest != "" {
		t.Fatal("failed write recorded delivery")
	}
	blocked := s
	blocked.Rules = []RuleEvidence{{Required: true, Status: "failed"}}
	if _, err := emitInput(&bytes.Buffer{}, blocked, "reviewer"); err == nil {
		t.Fatal("required failure emitted")
	}
	s.Rules[0].Bytes = []byte("tampered")
	if _, err := emitInput(&bytes.Buffer{}, s, "reviewer"); err == nil {
		t.Fatal("tampered content emitted")
	}
	if a.Digest != bytesDigest(reviewer.Bytes()) || b.Digest != bytesDigest(fixer.Bytes()) || a.Digest == b.Digest {
		t.Fatal("incorrect exact input identity")
	}
}
