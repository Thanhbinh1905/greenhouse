package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type reviewCapture struct {
	ID        string
	Dir       string
	Input     []byte
	Argv      []byte
	Events    []byte
	Timing    shimTiming
	HasInput  bool
	HasArgv   bool
	HasEvents bool
	HasTiming bool
}

func readReviewCaptures(root string) ([]reviewCapture, error) {
	reviews := filepath.Join(root, "reviews")
	entries, err := os.ReadDir(reviews)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	captures := make([]reviewCapture, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		capture := reviewCapture{ID: entry.Name(), Dir: filepath.Join(reviews, entry.Name())}
		capture.Input, err = os.ReadFile(filepath.Join(capture.Dir, "input.json"))
		capture.HasInput = err == nil
		capture.Argv, err = os.ReadFile(filepath.Join(capture.Dir, "argv.json"))
		capture.HasArgv = err == nil
		capture.Events, err = os.ReadFile(filepath.Join(capture.Dir, "events.jsonl"))
		capture.HasEvents = err == nil
		if timing, err := os.ReadFile(filepath.Join(capture.Dir, "timing.json")); err == nil && json.Unmarshal(timing, &capture.Timing) == nil && !capture.Timing.Start.IsZero() && !capture.Timing.End.IsZero() {
			capture.HasTiming = true
		}
		captures = append(captures, capture)
	}
	return captures, nil
}

func expectedReviewCaptureIDs(review map[string]any, passes int) []string {
	if digests := reviewInputDigests(review); len(digests) > 0 {
		ids := make([]string, 0, len(digests))
		for id := range digests {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids
	}
	id, _ := review["id"].(string)
	if id == "" {
		return nil
	}
	ids := map[string]bool{}
	reviewerExecutions := 0
	shardReview, sharded := review["shards"].(map[string]any)
	if sharded {
		if planner, _ := shardReview["planner"].(string); planner != "" {
			ids[id+"-plan"] = true
		}
		addExecutions := func(value any) {
			items, _ := value.([]any)
			for _, item := range items {
				if execution, ok := item.(string); ok && execution != "" {
					ids[execution] = true
					reviewerExecutions++
				}
			}
		}
		shards, _ := shardReview["shards"].([]any)
		for _, item := range shards {
			shard, _ := item.(map[string]any)
			addExecutions(shard["executions"])
		}
		boundary, _ := shardReview["boundary"].(map[string]any)
		addExecutions(boundary["executions"])
	}
	if !sharded || reviewerExecutions == 0 {
		if passes < 1 {
			passes = 1
		}
		for i := range passes {
			if passes == 1 {
				ids[id] = true
			} else {
				ids[fmt.Sprintf("%s-pass-%d", id, i+1)] = true
			}
		}
	}
	out := make([]string, 0, len(ids))
	for execution := range ids {
		out = append(out, execution)
	}
	sort.Strings(out)
	return out
}

func reviewInputDigests(review map[string]any) map[string]string {
	raw, ok := review["input_digests"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	digests := make(map[string]string, len(raw))
	for id, value := range raw {
		digest, ok := value.(string)
		if !ok || id == "" || digest == "" {
			return nil
		}
		digests[id] = digest
	}
	return digests
}

func reviewCapturesVerified(review map[string]any, captures []reviewCapture, passes int, input []byte) bool {
	expectedDigests := reviewInputDigests(review)
	expected := expectedReviewCaptureIDs(review, passes)
	if len(captures) == 0 || len(expected) == 0 || len(captures) != len(expected) || len(expectedDigests) != len(expected) {
		return false
	}
	byID := make(map[string]reviewCapture, len(captures))
	for _, capture := range captures {
		if _, duplicate := byID[capture.ID]; duplicate {
			return false
		}
		byID[capture.ID] = capture
	}
	for _, id := range expected {
		capture, ok := byID[id]
		if !ok || !capture.HasInput || !capture.HasArgv || !capture.HasEvents || runner.BytesDigest(capture.Input) != expectedDigests[id] {
			return false
		}
	}
	if len(expected) == 1 && expected[0] == review["id"] {
		return bytes.Equal(byID[expected[0]].Input, input)
	}
	return true
}

func modelFindingsFromAnswers(answers map[string]string) json.RawMessage {
	keys := make([]string, 0, len(answers))
	for key := range answers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var findings []json.RawMessage
	for _, key := range keys {
		raw := modelFindings(answers[key])
		if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
			continue
		}
		var part []json.RawMessage
		if json.Unmarshal(raw, &part) == nil {
			findings = append(findings, part...)
		}
	}
	if findings == nil {
		return json.RawMessage("null")
	}
	out, _ := json.Marshal(findings)
	return out
}

func copyCaptureFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
}
