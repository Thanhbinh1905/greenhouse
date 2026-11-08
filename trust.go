package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const overrideName = ".greenhouse.json"
const overrideVersion = 1

const (
	sourceBuiltIn    = "built-in default"
	sourcePerUser    = "per-user"
	sourceRepository = "trusted repository override"
	sourceRunLocal   = "run-local"
)

var settingFields = []string{"mode", "preset", "limit", "rules", "reviewer_backend", "reviewer_model", "fixer_backend", "fixer_model", "auto_resume"}

var authorityKeys = map[string]bool{"executable": true, "checks": true, "test": true, "lint": true, "docs": true, "commands": true, "grants": true, "repairs": true, "repair_limit": true, "remote": true, "required_checks": true, "yolo": true, "providers": true, "provider": true, "fallback": true, "claude": true, "codex": true, "name": true, "email": true}

type Trust struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Digest     string `json:"digest"`
	Actor      string `json:"actor"`
	Reason     string `json:"reason"`
	Action     string `json:"action_id"`
}

type RepositoryStatus struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Digest  string `json:"digest"`
	Trusted *Trust `json:"trusted,omitempty"`
}

func settingValues(s Settings) map[string]string {
	b, _ := json.Marshal(s)
	var fields map[string]json.RawMessage
	json.Unmarshal(b, &fields)
	out := map[string]string{}
	for _, name := range settingFields {
		out[name] = string(fields[name])
	}
	return out
}

func uniformSources(source string) map[string]string {
	out := map[string]string{}
	for _, name := range settingFields {
		out[name] = source
	}
	return out
}

func fieldSources(s Snapshot) map[string]string {
	if s.Sources != nil {
		out := map[string]string{}
		for name, source := range s.Sources {
			out[name] = source
		}
		return out
	}
	switch s.Source {
	case sourceBuiltIn:
		return uniformSources(sourceBuiltIn)
	case "saved", sourcePerUser:
		return uniformSources(sourcePerUser)
	}
	return uniformSources(sourceRunLocal)
}

func nextSnapshot(prev Snapshot, settings Settings, source string) Snapshot {
	sources := fieldSources(prev)
	before, after := settingValues(prev.Settings), settingValues(settings)
	for _, name := range settingFields {
		if before[name] != after[name] {
			sources[name] = sourceRunLocal
		}
	}
	return Snapshot{ID: opaque(), Settings: settings, Source: source, Sources: sources}
}

func userSources(s *Store) map[string]string {
	if s.DefaultsSource == sourceBuiltIn {
		return uniformSources(sourceBuiltIn)
	}
	return uniformSources(sourcePerUser)
}

func readOverride(worktree string) (string, []byte, error) {
	path := filepath.Join(worktree, overrideName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil, nil
	}
	if err != nil {
		return path, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16384 {
		return path, []byte{}, errors.New("the override must be a regular file of at most 16 KiB")
	}
	b, err := os.ReadFile(path)
	return path, b, err
}

func overrideDigest(b []byte) string {
	if b == nil {
		return "absent"
	}
	return bytesDigest(b)
}

func repositoryDefaults(s *Store, id Identity) (Settings, map[string]string, RepositoryStatus) {
	settings, sources := s.Defaults, userSources(s)
	path, b, err := readOverride(id.Worktree)
	status := RepositoryStatus{Path: path, Digest: overrideDigest(b)}
	if trust, ok := s.Trust[id.Repository]; ok {
		status.Trusted = &trust
	}
	switch {
	case err != nil:
		status.Status, status.Reason = "ignored", err.Error()
	case b == nil:
		status.Status = "absent"
	case status.Trusted == nil:
		status.Status, status.Reason = "ignored", "repository not trusted; trust it to apply this file to new runs"
	case status.Trusted.Digest != status.Digest:
		status.Status, status.Reason = "ignored", "file changed since it was trusted; trust it again to apply it"
	default:
		narrowed, fields, err := parseOverride(b, s.Defaults)
		if err != nil {
			status.Status, status.Reason = "ignored", err.Error()
			break
		}
		status.Status, settings = "applied", narrowed
		for _, name := range fields {
			sources[name] = sourceRepository
		}
	}
	return settings, sources, status
}

var modeRank = map[string]int{"ask": 0, "auto": 1, "yolo": 2}

func parseOverride(b []byte, base Settings) (Settings, []string, error) {
	version, fields, err := configVersion(overrideName, b)
	if err != nil {
		return base, nil, err
	}
	if _, ok := fields["version"]; !ok {
		return base, nil, errors.New(overrideName + ": version is required")
	}
	if version > overrideVersion {
		return base, nil, fmt.Errorf("%s: schema version %d is newer than this Greenhouse supports (%d)", overrideName, version, overrideVersion)
	}
	delete(fields, "version")
	if _, limited := fields["limit"]; limited || fields["budget_unit"] != nil {
		if err := budgetUnit(overrideName, fields); err != nil {
			return base, nil, err
		}
	}
	delete(fields, "budget_unit")
	var names []string
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if authorityKeys[name] {
			return base, nil, fmt.Errorf("repository overrides cannot set executable or approval authority (%q)", name)
		}
		if !has(settingFields, name) {
			return base, nil, fmt.Errorf("%s: unknown field %q", overrideName, name)
		}
	}
	settings := base
	settings.Rules = append([]string(nil), base.Rules...)
	merged, _ := json.Marshal(fields)
	if err := json.Unmarshal(merged, &settings); err != nil {
		return base, nil, fmt.Errorf("%s: %v", overrideName, err)
	}
	if err := narrows(settings, base); err != nil {
		return base, nil, err
	}
	return settings, names, nil
}

func narrows(s, base Settings) error {
	if rank, ok := modeRank[s.Mode]; !ok || rank > modeRank[base.Mode] {
		return fmt.Errorf("mode %s would expand approval authority beyond the per-user mode %s", s.Mode, base.Mode)
	}
	if s.Limit > base.Limit {
		return fmt.Errorf("limit %d is above the per-user limit %d; an override can only narrow it", s.Limit, base.Limit)
	}
	for _, rule := range s.Rules {
		name := strings.TrimPrefix(rule, "?")
		selected := builtins[name] != ""
		for _, own := range base.Rules {
			selected = selected || strings.TrimPrefix(own, "?") == name
		}
		if !selected {
			return fmt.Errorf("rule %q is neither built in nor selected in the per-user defaults", rule)
		}
	}
	for _, backend := range []string{s.ReviewerBackend, s.FixerBackend} {
		if backend == backendClaude || backend == backendCodex {
			config, err := trustedBackends()
			if err != nil || backend == backendClaude && config.Claude == nil || backend == backendCodex && config.Codex == nil {
				return fmt.Errorf("%s backend is not configured per user; an override can only select configured backends", backend)
			}
		}
	}
	if s.AutoResume == "on" && base.AutoResume == "off" {
		return errors.New("auto_resume on would expand the per-user choice off")
	}
	if !valid(s) {
		return errors.New(overrideName + ": invalid settings")
	}
	return nil
}

func overrideContent(settings, base Settings) []byte {
	values, defaults := settingValues(settings), settingValues(base)
	var out strings.Builder
	fmt.Fprintf(&out, "{\n  \"version\": %d", overrideVersion)
	if values["limit"] != defaults["limit"] {
		out.WriteString(",\n  \"budget_unit\": \"rounds\"")
	}
	for _, name := range settingFields {
		if values[name] != defaults[name] && values[name] != "" {
			fmt.Fprintf(&out, ",\n  %q: %s", name, values[name])
		}
	}
	out.WriteString("\n}\n")
	return []byte(out.String())
}

func saveRepositoryOverride(s *Store, id Identity, settings Settings, actor, reason, action string) error {
	_, _, status := repositoryDefaults(s, id)
	if status.Trusted == nil || status.Trusted.Digest != status.Digest {
		return errors.New("trust this repository and its current override file before saving to it")
	}
	content := overrideContent(settings, s.Defaults)
	if _, _, err := parseOverride(content, s.Defaults); err != nil {
		return err
	}
	if err := replaceFile(status.Path, content, 0644); err != nil {
		return err
	}
	s.Trust[id.Repository] = Trust{Repository: id.Repository, Path: status.Path, Digest: bytesDigest(content), Actor: actor, Reason: "saved: " + reason, Action: action}
	return nil
}

func initialSnapshot(s *Store, id Identity) Snapshot {
	settings, sources, status := repositoryDefaults(s, id)
	source := s.DefaultsSource
	if status.Status == "applied" {
		source = sourceRepository
	}
	return Snapshot{ID: opaque(), Settings: settings, Source: source, Sources: sources}
}

func sourceSummary(sources map[string]string) string {
	var order []string
	grouped := map[string][]string{}
	for _, name := range settingFields {
		source := sources[name]
		if grouped[source] == nil {
			order = append(order, source)
		}
		grouped[source] = append(grouped[source], name)
	}
	parts := make([]string, 0, len(order))
	for _, source := range order {
		parts = append(parts, source+": "+strings.Join(grouped[source], ", "))
	}
	return strings.Join(parts, " | ")
}

func settingsTable(s Snapshot) string {
	values, sources := settingValues(s.Settings), fieldSources(s)
	var lines []string
	for _, name := range settingFields {
		value := strings.Trim(values[name], `"`)
		if name == "rules" {
			value = strings.Join(s.Settings.Rules, ", ")
		}
		if value == "" {
			value = "default"
		}
		lines = append(lines, "  "+name+" = "+value+" ("+sources[name]+")")
	}
	return strings.Join(lines, "\n")
}

func overrideLine(o *RepositoryStatus) string {
	switch o.Status {
	case "absent":
		return "Repository override: none (no " + overrideName + ")"
	case "applied":
		return "Repository override: " + overrideName + " applied to new runs"
	}
	return "Repository override: " + overrideName + " IGNORED: " + o.Reason
}
