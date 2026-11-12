package engine

import (
	"errors"
	"reflect"
)

func validateRuleSnapshots(r *Run) error {
	seen := map[string]bool{}
	for _, s := range r.RuleSnapshots {
		var cfg *Snapshot
		for i := range r.Configs {
			if r.Configs[i].ID == s.ConfigID {
				cfg = &r.Configs[i]
				break
			}
		}
		if !validText(s.Scope) || s.ID == "" || seen[s.ID] || s.Candidate == "" || cfg == nil || !snapshotDigestMatches(*cfg, s.ConfigDigest) {
			return errors.New("invalid rule snapshot identity")
		}
		seen[s.ID] = true
		selected := selectedRules(cfg.Settings)
		if len(selected) != len(s.Rules) {
			return errors.New("invalid rule snapshot selections")
		}
		packs := map[string]bool{}
		for i, e := range s.Rules {
			base := selected[i]
			if e.Selection != base.Selection || e.Source != base.Source || e.Required != base.Required {
				return errors.New("invalid rule source")
			}
			if len(e.Bytes) > maxPackBytes || (len(e.Bytes) > 0 && e.Digest != bytesDigest(e.Bytes)) {
				return errors.New("invalid rule content digest")
			}
			switch e.Status {
			case "resolved":
				p, err := parsePack(e.Bytes)
				if err != nil || e.Error != "" || !reflect.DeepEqual(p, e.Pack) || packs[p.ID] {
					return errors.New("invalid resolved rule pack")
				}
				packs[p.ID] = true
			case "failed":
				if e.Error == "" || e.Pack != nil {
					return errors.New("invalid failed rule pack")
				}
			default:
				return errors.New("invalid durable rule status")
			}
		}
	}
	return nil
}
