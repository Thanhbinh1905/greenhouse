package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

type reviewRefusal struct{ Code, Message string }

func (e *reviewRefusal) Error() string { return e.Message }
func refusalResponse(v Response, err error) Response {
	v.Outcome, v.Error = "refused", err.Error()
	var refusal *reviewRefusal
	if errors.As(err, &refusal) {
		v.ErrorCode = refusal.Code
		retryable := false
		v.Retryable = &retryable
	}
	return v
}

func treeEntry(worktree, head, path string) (mode, object string, err error) {
	b, err := repository.Read(worktree, "--literal-pathspecs", "ls-tree", "-z", head, "--", path)
	if err != nil {
		return "", "", err
	}
	for _, entry := range strings.Split(string(b), "\x00") {
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if ok && name == path && len(fields) == 3 {
			return fields[0], fields[2], nil
		}
	}
	return "", "", nil
}

func findingContent(worktree, head, path string) ([]byte, error) {
	mode, pin, err := treeEntry(worktree, head, path)
	if err != nil {
		return nil, err
	}
	if mode == "160000" {
		return []byte("Subproject commit " + pin + "\n"), nil
	}
	return repository.ReadBounded(worktree, reviewBlobLimit, "cat-file", "blob", head+":"+path)
}

type reviewSubmodule struct {
	Path, Old, New, Log, Diff string
	Truncated                 bool
}

type reviewSubmoduleMetadata struct {
	Path, Old, New string
	Truncated      bool
}

func reviewSubmoduleHeaders(modules []reviewSubmodule) []reviewSubmoduleMetadata {
	headers := make([]reviewSubmoduleMetadata, len(modules))
	for i, module := range modules {
		headers[i] = reviewSubmoduleMetadata{module.Path, module.Old, module.New, module.Truncated}
	}
	return headers
}

func renderReviewSubmodules(modules []reviewSubmodule) []byte {
	var rendered bytes.Buffer
	if len(modules) == 0 {
		return nil
	}
	rendered.WriteString("\nSubmodules:\n")
	for _, module := range modules {
		fmt.Fprintf(&rendered, "--- %s ---\nOld: %s\nNew: %s\nTruncated: %t\nLog:\n%s\nDiff:\n%s\n", module.Path, module.Old, module.New, module.Truncated, module.Log, module.Diff)
	}
	return rendered.Bytes()
}

func refuseBinaryDiff(diff string) error {
	for _, section := range strings.Split(diff, "diff --git ") {
		if !strings.Contains(section, "\nGIT binary patch\n") {
			continue
		}
		path := strings.SplitN(section, "\n", 2)[0]
		return &reviewRefusal{"unsupported_binary_diff", "binary review unsupported: " + path}
	}
	return nil
}

func submoduleLocations(worktree, candidate, path string) []string {
	locations := []string{filepath.Join(worktree, path)}
	if b, err := os.ReadFile(filepath.Join(worktree, path, ".git")); err == nil {
		if gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: "); ok {
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(worktree, path, gitdir)
			}
			locations = append(locations, gitdir)
		}
	}
	b, err := repository.Read(worktree, "config", "--blob", candidate+":.gitmodules", "--null", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		return locations
	}
	common, err := repository.Read(worktree, "rev-parse", "--git-common-dir")
	if err != nil {
		return locations
	}
	root := strings.TrimSpace(string(common))
	if !filepath.IsAbs(root) {
		root = filepath.Join(worktree, root)
	}
	for _, entry := range strings.Split(string(b), "\x00") {
		key, value, ok := strings.Cut(entry, "\n")
		if !ok || value != path {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "submodule."), ".path")
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) {
			continue
		}
		locations = append(locations, filepath.Join(root, "modules", name))
	}
	return locations
}

func reviewSubmodules(worktree, base, candidate string, names []string) ([]reviewSubmodule, error) {
	var modules []reviewSubmodule
	rangeBytes := 0
	for _, path := range names {
		mode, newPin, err := treeEntry(worktree, candidate, path)
		if err != nil {
			return nil, err
		}
		oldMode, oldPin, err := treeEntry(worktree, base, path)
		if err != nil {
			return nil, err
		}
		if mode != "160000" && oldMode != "160000" {
			continue
		}
		if mode != "160000" {
			newPin = ""
		}
		if oldMode != "160000" {
			oldPin = ""
		}
		m := reviewSubmodule{Path: path, Old: oldPin, New: newPin}
		if oldMode != "160000" || mode != "160000" {
			modules = append(modules, m)
			continue
		}
		found := ""
		for _, location := range submoduleLocations(worktree, candidate, path) {
			good := true
			for _, pin := range []string{oldPin, newPin} {
				if pin == "" {
					continue
				}
				if _, err := repository.Read(location, "cat-file", "-e", pin+"^{commit}"); err != nil {
					good = false
					break
				}
			}
			if good {
				found = location
				break
			}
		}
		if found == "" {
			return nil, &reviewRefusal{"submodule_unavailable", fmt.Sprintf("submodule unavailable: path=%s old=%s new=%s; both pins must be local", path, oldPin, newPin)}
		}
		stats, err := repository.Read(found, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--numstat", "-z", oldPin, newPin, "--")
		if err != nil {
			if errors.Is(err, repository.ErrReadLimit) {
				return nil, &reviewRefusal{"review_input_too_large", "submodule change listing exceeds 1 MiB: " + path}
			}
			return nil, err
		}
		for _, entry := range strings.Split(string(stats), "\x00") {
			if strings.HasPrefix(entry, "-\t-\t") {
				return nil, &reviewRefusal{"unsupported_binary_diff", "binary review unsupported in submodule " + path + ": " + strings.TrimPrefix(entry, "-\t-\t")}
			}
		}
		log, err := repository.ReadBounded(found, 128*1024, "log", "--no-color", "--oneline", oldPin+".."+newPin, "--")
		if err != nil && !errors.Is(err, repository.ErrReadLimit) {
			return nil, err
		}
		m.Log = string(log)
		m.Truncated = errors.Is(err, repository.ErrReadLimit)
		diff, err := repository.ReadBounded(found, 128*1024, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-color", oldPin, newPin, "--")
		if err != nil && !errors.Is(err, repository.ErrReadLimit) {
			return nil, err
		}
		if e := refuseBinaryDiff(string(diff)); e != nil {
			return nil, e
		}
		if strings.Contains(string(diff), "\nBinary files ") {
			return nil, &reviewRefusal{"unsupported_binary_diff", "binary review unsupported in submodule " + path}
		}
		m.Diff = string(diff)
		m.Truncated = m.Truncated || errors.Is(err, repository.ErrReadLimit)
		if m.Truncated {
			m.Diff += "\n... [submodule range truncated]\n"
		}
		rangeBytes += len(m.Log) + len(m.Diff)
		if rangeBytes > reviewBundleLimit {
			return nil, &reviewRefusal{"review_input_too_large", fmt.Sprintf("review input too large: submodules=%d bytes at %s; diff, files, criteria, responses and total not yet measured", rangeBytes, path)}
		}
		modules = append(modules, m)
	}
	return modules, nil
}

const (
	reviewBlobLimit   = 16 * 1024 * 1024
	reviewFileLimit   = 64 * 1024
	reviewFilesLimit  = 768 * 1024
	reviewBundleLimit = 2 * 1024 * 1024
)

func numberedWindow(content []byte, spans [][2]int, limit int) string {
	var out strings.Builder
	omitted := false
	i, spanIndex := 0, 0
	for line := range strings.SplitAfterSeq(string(content), "\n") {
		i++
		if line == "" {
			continue
		}
		include := spans == nil
		for spanIndex < len(spans) && i > spans[spanIndex][1]+3 {
			spanIndex++
		}
		if spanIndex < len(spans) && i >= spans[spanIndex][0]-3 {
			include = true
		}
		if !include {
			if !omitted {
				out.WriteString("...\n")
			}
			omitted = true
			continue
		}
		omitted = false
		fmt.Fprintf(&out, "%d: %s\n", i, strings.TrimSuffix(line, "\n"))
		if out.Len() > limit {
			break
		}
	}
	return out.String()
}

func inputTooLarge(diff, files, criteria, responses, total int, paths []reviewFile) *reviewRefusal {
	ranked := append([]reviewFile(nil), paths...)
	fileBytes := func(file reviewFile) int {
		return len("--- ") + len(file.Path) + len(" ---\n") + len(file.Lines)
	}
	sort.Slice(ranked, func(i, j int) bool { return fileBytes(ranked[i]) > fileBytes(ranked[j]) })
	var largest []string
	for _, file := range ranked[:min(5, len(ranked))] {
		largest = append(largest, fmt.Sprintf("%s=%d", file.Path, fileBytes(file)))
	}
	return &reviewRefusal{"review_input_too_large", fmt.Sprintf("review input too large: diff=%d files=%d criteria=%d responses=%d total=%d; largest paths: %s", diff, files, criteria, responses, total, strings.Join(largest, ", "))}
}

func checkReviewInput(input []byte, diff string, files []reviewFile, criteria []byte, responses []reviewResponse, modules []reviewSubmodule) error {
	if len(input) <= reviewBundleLimit {
		return nil
	}
	fileBytes := 0
	for _, file := range files {
		fileBytes += len("--- ") + len(file.Path) + len(" ---\n") + len(file.Lines)
	}
	responseBytes := 0
	if len(responses) > 0 {
		encoded, _ := json.Marshal(responses)
		responseBytes = len(encoded)
	}
	err := inputTooLarge(len(diff), fileBytes, len(bytes.TrimSpace(criteria)), responseBytes, len(input), files)
	if len(modules) > 0 {
		metadataBytes, _ := json.Marshal(reviewSubmoduleHeaders(modules))
		err.Message += fmt.Sprintf("; submodules=%d", len(renderReviewSubmodules(modules))+len(metadataBytes))
	}
	return err
}
