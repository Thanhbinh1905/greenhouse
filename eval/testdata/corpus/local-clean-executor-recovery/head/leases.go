package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RefLease struct {
	Ref   string `json:"ref"`
	Owner string `json:"owner"`
	Home  string `json:"home"`
	Epoch int    `json:"epoch"`
}

func leaseOwner() (string, string, error) {
	h, err := home()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(h, "instance")
	id, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err = writeFileDurably(path, []byte(opaque())); err != nil {
			return "", "", err
		}
		id, err = os.ReadFile(path)
	}
	if err != nil {
		return "", "", err
	}
	return digest([]string{h, strings.TrimSpace(string(id))}), h, nil
}

func leasePath(repository, ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return filepath.Join(repository, "greenhouse-leases", hex.EncodeToString(sum[:16])+".json")
}

func readLease(repository, ref string) (*RefLease, error) {
	data, err := os.ReadFile(leasePath(repository, ref))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l RefLease
	if err = json.Unmarshal(data, &l); err != nil || l.Ref != ref {
		return nil, errors.New("unreadable ref lease for " + ref)
	}
	return &l, nil
}

func writeLease(repository string, l RefLease) error {
	if err := os.MkdirAll(filepath.Join(repository, "greenhouse-leases"), 0700); err != nil {
		return err
	}
	data, _ := json.Marshal(l)
	return writeFileDurably(leasePath(repository, l.Ref), data)
}

func checkLease(repository, ref string, epoch int) error {
	owner, _, err := leaseOwner()
	if err != nil {
		return err
	}
	l, err := readLease(repository, ref)
	switch {
	case err != nil:
		return err
	case l == nil && epoch == 0:
		return nil
	case l == nil:
		return fmt.Errorf("candidate ref %s has no lease; recovery required", ref)
	case l.Owner != owner:
		return fmt.Errorf("candidate ref %s is leased by another Greenhouse state home (%s)", ref, l.Home)
	case l.Epoch != epoch:
		return fmt.Errorf("stale lease for candidate ref %s: this run holds epoch %d, the lease is at epoch %d", ref, epoch, l.Epoch)
	}
	return nil
}

func acquireLease(repository, ref string, epoch int) (int, error) {
	l, err := readLease(repository, ref)
	if err != nil {
		return 0, err
	}
	if l != nil {
		return epoch, checkLease(repository, ref, epoch)
	}
	owner, h, err := leaseOwner()
	if err != nil {
		return 0, err
	}
	return 1, writeLease(repository, RefLease{Ref: ref, Owner: owner, Home: h, Epoch: 1})
}

func renewLease(repository, ref string, epoch int) (int, error) {
	if err := checkLease(repository, ref, epoch); err != nil {
		return 0, err
	}
	l, err := readLease(repository, ref)
	if err != nil || l == nil {
		return acquireLease(repository, ref, epoch)
	}
	l.Epoch++
	return l.Epoch, writeLease(repository, *l)
}

func checkCandidateLease(worktree, ref string, epoch int) error {
	id, err := resolve(worktree)
	if err != nil {
		return err
	}
	return checkLease(id.Repository, ref, epoch)
}

type heldLease struct{ ref, run string }

func leasesOf(r *Run) map[heldLease]int {
	held := map[heldLease]int{}
	if p := r.Pipeline; p != nil && p.Ref != "" {
		held[heldLease{p.Ref, r.ID}] = p.LeaseEpoch
	}
	if x := r.Repair; x != nil && x.Ref != "" {
		held[heldLease{x.Ref, r.ID}] = x.LeaseEpoch
	}
	return held
}

func leaseAlive(r *Run, ref string) bool {
	if rec := r.Recovery; rec != nil && !stopped(rec) {
		return true
	}
	if r.Status == "superseded" || r.Status == "discarded" || r.Status == "rethought" || r.Status == "restarted" {
		return false
	}
	if p := r.Pipeline; p != nil && p.Ref == ref {
		return !released(p) && p.Handback != "handed-back"
	}
	if x := r.Repair; x != nil && x.Ref == ref {
		return repairHeld(x)
	}
	return false
}

func releaseDeadLeases(s *Store) {
	owner, _, err := leaseOwner()
	if err != nil {
		return
	}
	repositories := map[string]bool{}
	for _, r := range s.Runs {
		repositories[r.Submitted.Repository] = true
	}
	for repository := range repositories {
		paths, _ := filepath.Glob(filepath.Join(repository, "greenhouse-leases", "*.json"))
		if len(paths) == 0 {
			continue
		}
		withWriterLock(repository, func() error {
			for _, path := range paths {
				data, err := os.ReadFile(path)
				var l RefLease
				if err != nil || json.Unmarshal(data, &l) != nil || l.Owner != owner || leasePath(repository, l.Ref) != path {
					continue
				}
				for _, r := range s.Runs {
					if epoch, ok := leasesOf(r)[heldLease{l.Ref, r.ID}]; ok && epoch == l.Epoch && !leaseAlive(r, l.Ref) {
						os.Remove(path)
					}
				}
			}
			return nil
		})
	}
}
