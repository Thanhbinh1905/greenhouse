package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type ResolutionResponse struct {
	Finding   string `json:"finding"`
	Response  string `json:"response"`
	Reason    string `json:"reason"`
	Authority string `json:"authority,omitempty"`
}

type ResolutionCheck struct {
	Finding string `json:"finding"`
	Check   string `json:"check"`
	Reason  string `json:"reason"`
}

type DeferralAuthority struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Choice string `json:"choice"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

const responseContract = `Finish with only a JSON object {"responses":[{"finding":"selected finding id","response":"fixed|disputed|deferred","reason":"what changed, or why not","authority":"Authorities id, deferred only"}]} holding exactly one response per Selected finding. Use fixed only after editing the candidate to resolve the finding, disputed when the finding is not a real defect, and deferred only when an Authorities entry covers postponing it. A dispute or deferral is not a fix; leave its lines unchanged. Edit only to resolve a finding you answer fixed, and change only the lines that resolution requires. Unselected findings are out of scope: leave every other line as it is, even one that seems to break a criterion or hold another defect. Criteria apply to the lines you change.`

const resolutionContract = ` Responses holds the fixer's answer to each prior finding. Return "resolutions":[{"finding":"prior finding id","check":"confirmed|rejected","reason":"why"}] with exactly one entry per response. For fixed, confirmed means the current candidate resolves the finding. For disputed, confirmed means the prior finding was not a real defect. For deferred, confirmed means the cited Decision covers postponing it. Report a defect that remains as a finding again.`

var resolutionKinds = map[string]string{"fixed": "fixed", "disputed": "dispute-upheld", "deferred": "deferred"}

// Automatic answers carry no authority to defer a finding.
func deferralAuthorities(r *Run) []DeferralAuthority {
	out := []DeferralAuthority{}
	if r.Pipeline == nil {
		return out
	}
	for _, d := range r.Pipeline.Decisions {
		if d.Answer != nil && d.Answer.Actor != "auto" {
			out = append(out, DeferralAuthority{ID: d.ID, Kind: d.Kind, Choice: d.Answer.Choice, Actor: d.Answer.Actor, Reason: d.Answer.Reason})
		}
	}
	return out
}

func parseResponses(text string, x *Repair) ([]ResolutionResponse, error) {
	fields, err := exactFields([]byte(text), "responses")
	if err != nil {
		return nil, errors.New("missing resolution response object")
	}
	var raw []json.RawMessage
	if json.Unmarshal(fields["responses"], &raw) != nil {
		return nil, errors.New("resolution responses must be an array")
	}
	selected := map[string]bool{}
	for _, f := range x.Selected {
		selected[f.ID] = true
	}
	authorities := map[string]bool{}
	for _, a := range x.Authorities {
		authorities[a.ID] = true
	}
	seen := map[string]bool{}
	var out []ResolutionResponse
	for _, item := range raw {
		if _, err := exactFields(item, "finding", "response", "reason", "authority"); err != nil {
			return nil, errors.New("unknown resolution response fields")
		}
		var r ResolutionResponse
		d := json.NewDecoder(strings.NewReader(string(item)))
		d.DisallowUnknownFields()
		if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
			return nil, errors.New("malformed resolution response")
		}
		switch {
		case !selected[r.Finding]:
			return nil, fmt.Errorf("resolution response names unselected finding %q", r.Finding)
		case seen[r.Finding]:
			return nil, fmt.Errorf("duplicate resolution response for %s", r.Finding)
		case resolutionKinds[r.Response] == "":
			return nil, fmt.Errorf("resolution response %q is not fixed, disputed or deferred", r.Response)
		case !validText(r.Reason):
			return nil, fmt.Errorf("resolution response for %s needs a reason", r.Finding)
		case r.Response == "deferred" && !authorities[r.Authority]:
			return nil, fmt.Errorf("deferred resolution response for %s cites no answered decision", r.Finding)
		case r.Response != "deferred" && r.Authority != "":
			return nil, fmt.Errorf("only a deferred resolution response cites an authority")
		}
		seen[r.Finding] = true
		out = append(out, r)
	}
	for id := range selected {
		if !seen[id] {
			return nil, fmt.Errorf("missing resolution response for %s", id)
		}
	}
	return out, nil
}

func fixedResponsesEdited(x *Repair) error {
	out, err := gitRead(x.Worktree, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return err
	}
	edited := map[string]bool{}
	for _, name := range strings.Split(string(out), "\x00") {
		edited[name] = true
	}
	files := map[string]string{}
	for _, f := range x.Selected {
		files[f.ID] = f.File
	}
	for _, r := range x.Responses {
		if r.Response == "fixed" && !edited[files[r.Finding]] {
			return fmt.Errorf("resolution response fixed for %s without editing %s", r.Finding, files[r.Finding])
		}
	}
	return nil
}

func anyFixed(responses []ResolutionResponse) bool {
	for _, r := range responses {
		if r.Response == "fixed" {
			return true
		}
	}
	return false
}

func pendingResponses(r *Run) (*Repair, []ResolutionResponse) {
	if r.Pipeline == nil {
		return nil, nil
	}
	for i := len(r.Pipeline.Repairs) - 1; i >= 0; i-- {
		x := &r.Pipeline.Repairs[i]
		if x.Stage != "review" {
			continue
		}
		if len(x.Responses) == 0 || responsesChecked(r.Pipeline, x) {
			return nil, nil
		}
		return x, x.Responses
	}
	return nil, nil
}

func responsesChecked(p *Pipeline, repair *Repair) bool {
	for _, review := range p.Reviews {
		if review.Result == nil || len(review.Responses) != len(repair.Responses) {
			continue
		}
		checked := map[string]bool{}
		for _, c := range review.Result.Resolutions {
			checked[c.Finding] = true
		}
		allChecked := true
		for _, response := range repair.Responses {
			allChecked = allChecked && checked[response.Finding]
		}
		if allChecked {
			return true
		}
	}
	return false
}

type reviewResponse struct {
	ResolutionResponse
	Prior    Finding            `json:"prior"`
	Decision *DeferralAuthority `json:"decision,omitempty"`
}

func reviewResponses(x *Repair) []reviewResponse {
	prior := map[string]Finding{}
	for _, f := range x.Selected {
		prior[f.ID] = f
	}
	var out []reviewResponse
	for _, resp := range x.Responses {
		item := reviewResponse{ResolutionResponse: resp, Prior: prior[resp.Finding]}
		for i := range x.Authorities {
			if x.Authorities[i].ID == resp.Authority {
				item.Decision = &x.Authorities[i]
			}
		}
		out = append(out, item)
	}
	return out
}

func validateResolutions(fields map[string]json.RawMessage, v *Verdict, x *Execution) error {
	raw, present := fields["resolutions"]
	if len(x.Responses) == 0 {
		if present {
			return errors.New("unexpected resolutions without responses")
		}
		return nil
	}
	var items []json.RawMessage
	if !present || json.Unmarshal(raw, &items) != nil {
		return errors.New("missing resolutions for fixer responses")
	}
	for _, item := range items {
		if _, err := exactFields(item, "finding", "check", "reason"); err != nil {
			return errors.New("unknown resolution check fields")
		}
	}
	want := map[string]bool{}
	for _, r := range x.Responses {
		want[r.Finding] = true
	}
	seen := map[string]bool{}
	for _, c := range v.Resolutions {
		if !want[c.Finding] || seen[c.Finding] || (c.Check != "confirmed" && c.Check != "rejected") || !validText(c.Reason) {
			return errors.New("invalid resolution check")
		}
		seen[c.Finding] = true
	}
	if len(seen) != len(want) {
		return errors.New("incomplete resolution checks")
	}
	return nil
}

func resolvedKinds(r *Run) map[string]string {
	out := map[string]string{}
	if r.Pipeline == nil {
		return out
	}
	labels := map[string]string{"": "fixed", "fixed": "fixed", "dispute-upheld": "dispute upheld", "deferred": "deferred"}
	for _, res := range r.Pipeline.Resolutions {
		if res.Reopened == "" {
			out[res.Finding] = labels[res.Kind]
		}
	}
	return out
}

func responseLines(r *Run) map[string][]string {
	out := map[string][]string{}
	var x *Repair
	if r.Pipeline != nil {
		for i := len(r.Pipeline.Repairs) - 1; i >= 0 && x == nil; i-- {
			if len(r.Pipeline.Repairs[i].Responses) > 0 {
				x = &r.Pipeline.Repairs[i]
			}
		}
	} else if r.Repair != nil && len(r.Repair.Responses) > 0 {
		x = r.Repair
	}
	if x == nil {
		return out
	}
	checks := map[string]ResolutionCheck{}
	if r.Review != nil && r.Review.Result != nil && len(r.Review.Responses) > 0 {
		for _, c := range r.Review.Result.Resolutions {
			checks[c.Finding] = c
		}
	}
	for _, resp := range x.Responses {
		line := "  Response " + resp.Response + ": " + resp.Reason
		if resp.Authority != "" {
			line += " | authority " + short(resp.Authority)
		}
		check := "  Review check pending"
		if c, ok := checks[resp.Finding]; ok {
			check = "  Review check " + c.Check + ": " + c.Reason
		} else if r.Pipeline == nil {
			check = "  Not independently checked: one-pass never rereviews"
		}
		out[resp.Finding] = append(out[resp.Finding], line, check)
	}
	return out
}
