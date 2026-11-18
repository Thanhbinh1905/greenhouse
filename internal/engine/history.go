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
	return archiveWithWriter(home, kind, id, value, writeFileDurably)
}

func archiveWithWriter(home, kind, id string, value any, write func(string, []byte) error) error {
	path := historyPath(home, kind, id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return write(path, data)
}

func historyGarbage(home, kind string) ([]string, error) {
	dir := filepath.Join(home, "history", kind)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]os.FileInfo, 0, len(entries))
	var size int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
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
	var garbage []string
	for len(files) > historyLimit || size > historyBytes {
		oldest := files[0]
		garbage = append(garbage, filepath.Join(dir, oldest.Name()))
		size -= oldest.Size()
		files = files[1:]
	}
	return garbage, nil
}

func compactHistory(home string, s *Store, write func(string, []byte) error) ([]string, error) {
	if s.ReceiptIndex == nil {
		s.ReceiptIndex = map[string]string{}
	}
	slots := map[string]bool{}
	for _, id := range s.Slots {
		slots[id] = true
	}
	ids := make([]string, 0, len(s.Runs))
	for id := range s.Runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := s.Runs[id]
		if r == nil || slots[id] || !hostReplaceable(r) || hasDurableLease(home, r) {
			continue
		}
		if err := archiveWithWriter(home, "runs", id, r, write); err != nil {
			return nil, err
		}
		delete(s.Runs, id)
	}
	ids = ids[:0]
	for id := range s.Receipts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := archiveWithWriter(home, "receipts", id, s.Receipts[id], write); err != nil {
			return nil, err
		}
		s.ReceiptIndex[id] = s.Receipts[id].Digest
		delete(s.Receipts, id)
	}
	var garbage []string
	for _, kind := range []string{"runs", "receipts"} {
		paths, err := historyGarbage(home, kind)
		if err != nil {
			return nil, err
		}
		garbage = append(garbage, paths...)
	}
	pruned := map[string]bool{}
	for _, path := range garbage {
		pruned[path] = true
	}
	for id := range s.ReceiptIndex {
		path := historyPath(home, "receipts", id)
		if pruned[path] {
			delete(s.ReceiptIndex, id)
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			delete(s.ReceiptIndex, id)
		} else if err != nil {
			return nil, err
		}
	}
	return garbage, nil
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

func hasDurableLease(home string, r *Run) bool {
	for held, epoch := range leasesOf(r) {
		lease, err := readLease(r.Submitted.Repository, held.ref)
		if err != nil || lease != nil && lease.Home == home && lease.Epoch == epoch {
			return true
		}
	}
	return false
}
