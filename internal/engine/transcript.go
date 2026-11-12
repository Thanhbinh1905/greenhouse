package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const transcriptLimit = 4 << 20
const transcriptOmitted = "\n[transcript limit reached; later output omitted]\n"

type TranscriptView struct {
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Limit   int    `json:"limit"`
	Content string `json:"content"`
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|token|secret|password|authorization)["']?\s*[:=]\s*["']?)[^"'\s,}]{8,}`),
}

func secretName(name string) bool {
	upper := strings.ToUpper(name)
	for _, word := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH", "COOKIE", "SESSION"} {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

func Redact(b []byte) []byte {
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		if secretName(name) && len(value) >= 8 {
			b = bytes.ReplaceAll(b, []byte(value), []byte("[REDACTED:"+name+"]"))
		}
	}
	for _, pattern := range secretPatterns {
		b = pattern.ReplaceAll(b, []byte("${1}[REDACTED]"))
	}
	return b
}

func transcriptName(run string) string { return filepath.Join("transcripts", run+".log") }

func appendTranscript(run, header string, stdout, stderr []byte) {
	h, err := Home()
	if err != nil || run == "" {
		return
	}
	if os.MkdirAll(filepath.Join(h, "transcripts"), 0700) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(h, transcriptName(run)), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() >= transcriptLimit {
		return
	}
	var section bytes.Buffer
	section.WriteString("=== " + header + " ===\n--- stdout ---\n")
	section.Write(stdout)
	section.WriteString("\n--- stderr ---\n")
	section.Write(stderr)
	section.WriteString("\n")
	out := Redact(section.Bytes())
	if room := transcriptLimit - len(transcriptOmitted) - int(info.Size()); len(out) > room {
		out = append(out[:max(room, 0)], transcriptOmitted...)
	}
	f.Write(out)
}

func readTranscript(run string) (*TranscriptView, error) {
	h, err := Home()
	if err != nil {
		return nil, err
	}
	view := &TranscriptView{Path: transcriptName(run), Limit: transcriptLimit}
	b, err := os.ReadFile(filepath.Join(h, view.Path))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	view.Bytes, view.Content = len(b), string(b)
	return view, nil
}
