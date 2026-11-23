package engine

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

type GeneratedPath struct {
	Path       string   `json:"path"`
	Added      int      `json:"added"`
	Removed    int      `json:"removed"`
	Generators []string `json:"generators,omitempty"`
}

type UncoveredPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

const generatedExcerptLimit = 4 * 1024

const generatedContract = ` Generated lists changed paths that the target branch marks as generated. Their Diff hunks are omitted and Files holds only a bounded excerpt of each, plus the content of each named generator that exists at the candidate. Assess them from the diffstat, excerpt and generators, and include them in coverage.`

const uncoveredContract = ` Uncovered lists changed paths that were not reviewed. Their Diff hunks and Files content are omitted, Paths excludes them, and coverage must not include them. Reasons identify hunk_window, prompt_budget, or review_budget exclusions.`

func pathAttributes(worktree, source string, names []string) (map[string]map[string]string, error) {
	out, err := repository.Read(worktree, append([]string{"check-attr", "--source=" + source, "-z", "linguist-generated", "greenhouse-generator", "--"}, names...)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read target attributes: %w", err)
	}
	fields := strings.Split(string(out), "\x00")
	values := map[string]map[string]string{}
	for i := 0; i+2 < len(fields); i += 3 {
		if values[fields[i]] == nil {
			values[fields[i]] = map[string]string{}
		}
		values[fields[i]][fields[i+1]] = fields[i+2]
	}
	return values, nil
}

func trustedGenerated(worktree, target, base, candidate string, names []string) ([]GeneratedPath, error) {
	empty, err := repository.Read(worktree, "hash-object", "-t", "tree", "/dev/null")
	if err != nil {
		return nil, err
	}
	declared, err := pathAttributes(worktree, target, names)
	if err != nil {
		return nil, err
	}
	outsideTarget, err := pathAttributes(worktree, strings.TrimSpace(string(empty)), names)
	if err != nil {
		return nil, err
	}
	var generated []GeneratedPath
	for _, name := range names {
		if v := declared[name]["linguist-generated"]; (v != "set" && v != "true") || outsideTarget[name]["linguist-generated"] != "unspecified" {
			continue
		}
		entry := GeneratedPath{Path: name}
		if v := declared[name]["greenhouse-generator"]; outsideTarget[name]["greenhouse-generator"] == "unspecified" && v != "unspecified" && v != "set" && v != "unset" {
			for _, generator := range strings.Split(v, ",") {
				generator = path.Clean(generator)
				if generator != "." && !path.IsAbs(generator) && generator != ".." && !strings.HasPrefix(generator, "../") && !slices.Contains(entry.Generators, generator) {
					entry.Generators = append(entry.Generators, generator)
				}
			}
		}
		generated = append(generated, entry)
	}
	if len(generated) == 0 {
		return nil, nil
	}
	stats, err := repository.Read(worktree, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-relative", "--numstat", "-z", base, candidate, "--")
	if err != nil {
		return nil, err
	}
	for _, entry := range strings.Split(string(stats), "\x00") {
		parts := strings.SplitN(entry, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		for i := range generated {
			if generated[i].Path == parts[2] {
				generated[i].Added, _ = strconv.Atoi(parts[0])
				generated[i].Removed, _ = strconv.Atoi(parts[1])
			}
		}
	}
	return generated, nil
}

func boundedExcerpt(lines string, limit int) string {
	if len(lines) <= limit {
		return lines
	}
	cut := lines[:limit]
	if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
		cut = cut[:i+1]
	} else {
		cut = strings.ToValidUTF8(cut, "") + "\n"
	}
	return cut + "...\n"
}

func omitDiffHunks(worktree, base, candidate, diff, path, note string) (string, error) {
	section, err := repository.Read(worktree, "--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", "--no-color", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--binary", base, candidate, "--", path)
	if err != nil {
		return "", err
	}
	if len(section) == 0 || !strings.Contains(diff, string(section)) {
		return "", fmt.Errorf("cannot isolate the diff of %s", path)
	}
	header, _, found := strings.Cut(string(section), "\n@@ ")
	if !found {
		return diff, nil
	}
	return strings.Replace(diff, string(section), header+"\n"+note+"\n", 1), nil
}

func generatorFiles(worktree, candidate string, generated []GeneratedPath, files []reviewFile) ([]reviewFile, error) {
	for _, entry := range generated {
		for _, generator := range entry.Generators {
			if slices.ContainsFunc(files, func(f reviewFile) bool { return f.Path == generator }) {
				continue
			}
			mode, object, err := treeEntry(worktree, candidate, generator)
			if err != nil {
				return nil, err
			}
			if mode != "100644" && mode != "100755" {
				continue
			}
			content, err := repository.ReadBounded(worktree, reviewBlobLimit, "cat-file", "blob", object)
			if err != nil {
				return nil, fmt.Errorf("cannot read generator %s: %w", generator, err)
			}
			files = append(files, reviewFile{generator, boundedExcerpt(numberedWindow(content, nil, reviewFileLimit), reviewFileLimit)})
		}
	}
	return files, nil
}
