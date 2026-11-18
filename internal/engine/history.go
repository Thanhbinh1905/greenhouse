package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const historyLimit = 128
const historyBytes = 64 * 1024 * 1024

func historyPath(home, kind, id string) string {
	return filepath.Join(home, "history", kind, Digest(id)+".json")
}

func archive(home, kind, id string, value any) error {
	path := historyPath(home, kind, id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeFileDurably(path, data)
}

func trimHistory(home, kind string) error {
	dir := filepath.Join(home, "history", kind)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	files := make([]os.FileInfo, 0, len(entries))
	var size int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			continue
		}
		files = append(files, info)
		size += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime().Equal(files[j].ModTime()) {
			return files[i].Name() < files[j].Name()
		}
		return files[i].ModTime().Before(files[j].ModTime())
	})
	for len(files) > historyLimit || size > historyBytes {
		oldest := files[0]
		if err := os.Remove(filepath.Join(dir, oldest.Name())); err != nil {
			return err
		}
		size -= oldest.Size()
		files = files[1:]
	}
	return nil
}

func compactHistory(home string, s *Store) error {
	if s.ReceiptIndex == nil {
		s.ReceiptIndex = map[string]string{}
	}
	slots := map[string]bool{}
	for _, id := range s.Slots {
		slots[id] = true
	}
	wroteRuns, wroteReceipts := false, false
	ids := make([]string, 0, len(s.Runs))
	for id := range s.Runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := s.Runs[id]
		if r == nil || slots[id] || !hostReplaceable(r) {
			continue
		}
		if err := archive(home, "runs", id, r); err != nil {
			return err
		}
		delete(s.Runs, id)
		wroteRuns = true
	}
	ids = ids[:0]
	for id := range s.Receipts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := archive(home, "receipts", id, s.Receipts[id]); err != nil {
			return err
		}
		s.ReceiptIndex[id] = s.Receipts[id].Digest
		delete(s.Receipts, id)
		wroteReceipts = true
	}
	if wroteRuns {
		if err := trimHistory(home, "runs"); err != nil {
			return err
		}
	}
	if wroteReceipts {
		if err := trimHistory(home, "receipts"); err != nil {
			return err
		}
		for id := range s.ReceiptIndex {
			if _, err := os.Stat(historyPath(home, "receipts", id)); os.IsNotExist(err) {
				delete(s.ReceiptIndex, id)
			} else if err != nil {
				return err
			}
		}
	}
	return nil
}

func historicalRun(s *Store, id string) (*Run, error) {
	if r := s.Runs[id]; r != nil {
		return r, nil
	}
	if s.home == "" {
		return nil, nil
	}
	data, err := os.ReadFile(historyPath(s.home, "runs", id))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.ID != id || len(r.Configs) == 0 {
		return nil, fmt.Errorf("invalid historical run")
	}
	return &r, nil
}

func historicalReceipt(s *Store, id string) (Receipt, bool, error) {
	if r, ok := s.Receipts[id]; ok {
		return r, true, nil
	}
	if _, committed := s.ReceiptIndex[id]; !committed || s.home == "" || id == "" {
		return Receipt{}, false, nil
	}
	data, err := os.ReadFile(historyPath(s.home, "receipts", id))
	if os.IsNotExist(err) {
		return Receipt{}, false, fmt.Errorf("action receipt data was pruned; inspect the current run before retrying")
	}
	if err != nil {
		return Receipt{}, false, err
	}
	var receipt Receipt
	err = json.Unmarshal(data, &receipt)
	if err == nil && receipt.Digest != s.ReceiptIndex[id] {
		return Receipt{}, false, fmt.Errorf("action receipt digest mismatch")
	}
	return receipt, err == nil, err
}
