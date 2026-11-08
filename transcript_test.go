package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscriptLogIsRedactedBoundedAndViewable(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `sys.stderr.write('auth '+os.environ.get('GREENHOUSE_TEST_PROVIDER_KEY','missing')+' Bearer abcdefghijklmnopqrstuvwxyz0123\n')`)
	mutate(t, bin, repo, home, v, "review", "review", nil)
	reviewed := waitReview(t, bin, repo, home)
	if _, present := reviewed["transcript"]; present {
		t.Fatal("normal inspection carries transcript content")
	}
	fixer := filepath.Join(filepath.Dir(home), "fake-fixer")
	writeExecutable(t, fixer, "#!/usr/bin/python3\nimport json,sys\njson.load(sys.stdin)\nprint('fixer token ghp_abcdefghijklmnopqrstuvwxyz0123456789')\nopen('sample.txt','w').write('repaired\\n')\n")
	writePrivateJSON(t, filepath.Join(home, "fixer.json"), map[string]any{"executable": fixer, "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/true"}}})
	repairRequest(t, bin, repo, home, reviewed, "repair", "repair", selectedID(t, reviewed))
	repaired := waitRepair(t, bin, repo, home)
	if runOf(t, repaired)["status"] != "fixed-not-independently-rereviewed" {
		t.Fatalf("repair did not complete: %v", runOf(t, repaired)["repair"])
	}
	logged := axi(t, bin, repo, home, `{"version":1,"op":"transcript"}`)
	tr, ok := logged["transcript"].(map[string]any)
	if logged["outcome"] != "ok" || !ok {
		t.Fatalf("transcript unavailable: %v", logged)
	}
	content := tr["content"].(string)
	review := runOf(t, reviewed)["review"].(map[string]any)
	repair := runOf(t, repaired)["repair"].(map[string]any)
	for _, want := range []string{"reviewer " + review["id"].(string), "fixer " + repair["id"].(string), "auth [REDACTED", "Bearer [REDACTED]", "fixer token [REDACTED]"} {
		if !strings.Contains(content, want) {
			t.Fatalf("transcript lacks %q:\n%s", want, content)
		}
	}
	for _, secret := range []string{"fixture-secret", "abcdefghijklmnopqrstuvwxyz0123", "ghp_abcdef"} {
		if strings.Contains(content, secret) {
			t.Fatalf("transcript leaks %q:\n%s", secret, content)
		}
	}
	path := filepath.Join(home, tr["path"].(string))
	info, err := os.Stat(path)
	dir, _ := os.Stat(filepath.Dir(path))
	if err != nil || info.Mode().Perm() != 0600 || dir.Mode().Perm() != 0700 || tr["limit"] != float64(transcriptLimit) {
		t.Fatalf("transcript file not private and bounded: %v %v %v", info, dir, tr)
	}
}

func TestTranscriptNearLimitStaysBounded(t *testing.T) {
	home := filepath.Join(t.TempDir(), "state")
	t.Setenv("GREENHOUSE_HOME", home)
	os.MkdirAll(filepath.Join(home, "transcripts"), 0700)
	path := filepath.Join(home, "transcripts", "run.log")
	writeRaw(t, path, strings.Repeat("x", transcriptLimit-10))
	appendTranscript("run", "reviewer exec", []byte("more output"), nil)
	b, err := os.ReadFile(path)
	if err != nil || len(b) > transcriptLimit+len(transcriptOmitted) || !strings.HasSuffix(string(b), transcriptOmitted) {
		t.Fatalf("near-limit append not bounded or unmarked: %d %v", len(b), err)
	}
}

func TestTranscriptStopsAtPerRunLimit(t *testing.T) {
	home := filepath.Join(t.TempDir(), "state")
	t.Setenv("GREENHOUSE_HOME", home)
	big := []byte(strings.Repeat("x", transcriptLimit/3))
	for i := 0; i < 5; i++ {
		appendTranscript("run", "reviewer exec", big, nil)
	}
	b, err := os.ReadFile(filepath.Join(home, "transcripts", "run.log"))
	if err != nil || len(b) > transcriptLimit || !strings.HasSuffix(string(b), transcriptOmitted) {
		t.Fatalf("transcript exceeds its bound or lacks the omission marker: %d %v", len(b), err)
	}
}
