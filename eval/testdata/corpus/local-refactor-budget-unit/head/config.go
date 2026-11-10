package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const currentConfigVersion = 2

var configFiles = []string{"reviewer.json", "fixer.json", "providers.json", "backends.json", "pipeline.json"}

type schemaError struct{ message string }

func (e schemaError) Error() string { return e.message }

type ConfigStatus struct {
	File    string `json:"file"`
	Version int    `json:"version,omitempty"`
	Current int    `json:"current"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type Migration struct {
	File   string `json:"file"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Backup string `json:"backup"`
	Before string `json:"before"`
	After  string `json:"after"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Action string `json:"action_id"`
}

func configVersion(name string, b []byte) (int, map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(b))
	if d.Decode(&fields) != nil || fields == nil || d.Decode(new(any)) != io.EOF {
		return 0, nil, fmt.Errorf("%s is not one JSON object", name)
	}
	raw, ok := fields["version"]
	if !ok {
		return 1, fields, nil
	}
	var version int
	if json.Unmarshal(raw, &version) != nil || version < 1 {
		return 0, nil, schemaError{name + ": version must be a positive integer"}
	}
	return version, fields, nil
}

func budgetUnit(name string, fields map[string]json.RawMessage) error {
 raw,ok:=fields["budget_unit"]
 if !ok {return schemaError{name + ": budget_unit is required; the only supported unit is \"rounds\""}}
 var unit string
 json.Unmarshal(raw,&unit)
 switch unit {
 case "rounds":return nil
 default:return schemaError{fmt.Sprintf("%s: budget_unit %s is unsupported; Greenhouse counts rounds and never converts or reinterprets a budget unit",name,raw)}
 }
}

// decodeConfig strictly decodes a current-version per-user file into out.
func decodeConfig(name string, b []byte, out any) error {
	version, fields, err := configVersion(name, b)
	if err != nil {
		return err
	}
	if version > currentConfigVersion {
		return schemaError{fmt.Sprintf("%s: schema version %d is newer than this Greenhouse supports (%d); upgrade Greenhouse; the file was left unchanged", name, version, currentConfigVersion)}
	}
	if version < currentConfigVersion {
		return schemaError{fmt.Sprintf("%s uses schema version %d; migrate it to version %d (terminal u, AXI op migrate); a backup is kept", name, version, currentConfigVersion)}
	}
	delete(fields, "version")
	if name == "pipeline.json" {
		if err := budgetUnit(name, fields); err != nil {
			return err
		}
		delete(fields, "budget_unit")
	}
	rest, _ := json.Marshal(fields)
	d := json.NewDecoder(bytes.NewReader(rest))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func configFailure(err error, generic string) error {
	var schema schemaError
	if errors.As(err, &schema) {
		return err
	}
	return errors.New(generic)
}

func configStatuses() []ConfigStatus {
	h, err := home()
	out := []ConfigStatus{}
	if err != nil {
		return out
	}
	for _, name := range configFiles {
		status := ConfigStatus{File: name, Current: currentConfigVersion}
		b, err := os.ReadFile(filepath.Join(h, name))
		switch {
		case os.IsNotExist(err):
			status.Status = "absent"
		case err != nil:
			status.Status, status.Error = "invalid", err.Error()
		default:
			version, _, err := configVersion(name, b)
			status.Version = version
			switch {
			case err != nil:
				status.Status, status.Error = "invalid", err.Error()
			case version > currentConfigVersion:
				status.Status = "newer"
				status.Error = fmt.Sprintf("%s: schema version %d is newer than this Greenhouse supports (%d); upgrade Greenhouse", name, version, currentConfigVersion)
			case version < currentConfigVersion:
				status.Status = "older"
				status.Error = fmt.Sprintf("%s uses schema version %d; migrate it to version %d", name, version, currentConfigVersion)
			default:
				status.Status = "current"
			}
		}
		out = append(out, status)
	}
	return out
}

func legacyTarget(name string) any {
	switch name {
	case "reviewer.json":
		return &ReviewerConfig{}
	case "fixer.json":
		return &FixerConfig{}
	case "providers.json":
		return &TrustedProviderConfig{}
	case "backends.json":
		return &BackendsConfig{}
	}
	return &CommandPlan{}
}

type pendingMigration struct {
	record    Migration
	original  []byte
	migrated  []byte
	path, bak string
}

// migrateConfig upgrades every older per-user file, or changes nothing when any file cannot be migrated.
func migrateConfig(actor, reason, action string) ([]Migration, error) {
	h, err := home()
	if err != nil {
		return nil, err
	}
	var pending []pendingMigration
	for _, name := range configFiles {
		path := filepath.Join(h, name)
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		version, _, err := configVersion(name, b)
		if err != nil {
			return nil, err
		}
		if version > currentConfigVersion {
			return nil, fmt.Errorf("%s: schema version %d is newer than this Greenhouse supports (%d); upgrade Greenhouse; no file was changed", name, version, currentConfigVersion)
		}
		if version == currentConfigVersion {
			continue
		}
		before, after := legacyTarget(name), legacyTarget(name)
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(before) != nil {
			return nil, fmt.Errorf("%s: version 1 content is invalid; fix it before migration; no file was changed", name)
		}
		head := fmt.Sprintf(`{"version":%d,`, currentConfigVersion)
		if name == "pipeline.json" {
			head += `"budget_unit":"rounds",`
		}
		body := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(b), []byte("{")))
		migrated := append([]byte(head), body...)
		if bytes.Equal(body, []byte("}")) {
			migrated = append([]byte(head[:len(head)-1]), body...)
		}
		if decodeConfig(name, migrated, after) != nil || digest(before) != digest(after) {
			return nil, fmt.Errorf("%s: migration would not be lossless; no file was changed", name)
		}
		backup := fmt.Sprintf("%s.v%d-%s.bak", name, version, bytesDigest(b)[:12])
		pending = append(pending, pendingMigration{Migration{File: name, From: version, To: currentConfigVersion, Backup: backup, Before: bytesDigest(b), After: bytesDigest(migrated), Actor: actor, Reason: reason, Action: action}, b, migrated, path, filepath.Join(h, backup)})
	}
	var records []Migration
	for _, m := range pending {
		if err := writeBackup(m.bak, m.original); err != nil {
			return records, err
		}
		if err := replaceFile(m.path, m.migrated, 0600); err != nil {
			return records, err
		}
		records = append(records, m.record)
	}
	return records, nil
}

func writeBackup(path string, b []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, b) {
			return nil
		}
		return errors.New("backup " + filepath.Base(path) + " exists with different content")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func replaceFile(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".greenhouse-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		if _, err = f.Write(b); err == nil {
			err = f.Sync()
		}
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
