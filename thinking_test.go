package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const thinkingFakePi = `#!/usr/bin/python3
import sys,json,os
if '--list-models' in sys.argv:
 print('provider  model  context  max-out  thinking  images')
 print('fixture   model  128K     16K      yes       no')
 sys.exit(0)
root=os.path.dirname(__file__)
b=json.load(sys.stdin)
with open(root+'/models.json','w') as f: f.write(open(os.environ['PI_CODING_AGENT_DIR']+'/models.json').read())
v={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','thinkingLevel':'medium','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`

func TestPiReviewerRunsWithTheCatalogThinkingSetting(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	root := filepath.Dir(home)
	if err := os.WriteFile(filepath.Join(root, "fake-pi"), []byte(thinkingFakePi), 0700); err != nil {
		t.Fatal(err)
	}
	if started := mutate(t, bin, repo, home, v, "review", "review", nil); started["outcome"] != "ok" {
		t.Fatal(started)
	}
	run := runOf(t, waitReview(t, bin, repo, home))
	models, err := os.ReadFile(filepath.Join(root, "models.json"))
	if err != nil || !strings.Contains(string(models), `"reasoning":true`) {
		t.Fatalf("the provider entry drops the catalog's reasoning flag: %s %v", models, err)
	}
	if got := run["review"].(map[string]any)["thinking"]; got != "medium" {
		t.Fatalf("review thinking = %v, want the level Pi reported", got)
	}
	if got := run["attempts"].([]any)[0].(map[string]any)["thinking"]; got != "medium" {
		t.Fatalf("attempt thinking = %v", got)
	}
}

func TestPiFixerRecordsTheEffectiveThinkingLevel(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := repairFixture(t)
	root := filepath.Dir(home)
	script := `#!/usr/bin/python3
import sys,json,os
if '--list-models' in sys.argv:
 print('provider model context max-out thinking images')
 print('fixture model 128K 16K yes no')
 sys.exit(0)
b=json.load(sys.stdin)
with open(os.path.dirname(__file__)+'/fixer-models.json','w') as f: f.write(open(os.environ['PI_CODING_AGENT_DIR']+'/models.json').read())
with open('sample.txt','w') as f: f.write('repaired\n')
` + strings.Replace(respondFixed, "'stopReason':'stop'", "'stopReason':'stop','thinkingLevel':'medium'", 1)
	if err := os.WriteFile(filepath.Join(root, "fake-fixer"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if started := repairRequest(t, bin, repo, home, v, "repair", "repair", selectedID(t, v)); started["outcome"] != "ok" {
		t.Fatal(started)
	}
	run := runOf(t, waitRepair(t, bin, repo, home))
	repair := run["repair"].(map[string]any)
	if repair["status"] != "fixed-not-independently-rereviewed" || repair["thinking"] != "medium" {
		t.Fatalf("repair = %v", repair)
	}
	models, err := os.ReadFile(filepath.Join(root, "fixer-models.json"))
	if err != nil || !strings.Contains(string(models), `"reasoning":true`) {
		t.Fatalf("fixer reasoning missing: %s %v", models, err)
	}
	attempts := run["attempts"].([]any)
	if got := attempts[len(attempts)-1].(map[string]any)["thinking"]; got != "medium" {
		t.Fatalf("fixer attempt thinking = %v", got)
	}
}
