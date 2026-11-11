import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time

binary = str(Path(sys.argv[1]).resolve())
evidence = Path(sys.argv[2])
evidence.mkdir(parents=True, exist_ok=True)
root = Path(tempfile.mkdtemp(prefix="gh-tui-", dir=os.environ["TMPDIR"]))
socket = root / "tmux.sock"
color = re.compile(r"\x1b\[[0-9;]*(?:3[0-9]|4[0-9]|9[0-7]|10[0-7])(?:;[0-9;]*)?m")

fake_pi = '''#!/usr/bin/python3
import json,os,sys,time
root=os.path.dirname(os.path.realpath(__file__))
if '--list-models' in sys.argv:
    print('provider model')
    print('native default')
    sys.exit(0)
b=json.load(sys.stdin)
if any('read,edit,write' in a for a in sys.argv):
    open('sample.txt','w').write('repaired\\n')
    out={'responses':[{'finding':f['id'],'response':'fixed','reason':'rewrote the selected line'} for f in b.get('Selected') or []]}
else:
    if os.path.exists(root+'/delay'):
        time.sleep(float(open(root+'/delay').read()))
    out={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]}
    if '"documentation"' in b['Contract']:
        out['documentation']={'status':'not-needed','reason':'fixture changes no public behavior'}
    if open('sample.txt').read()!='repaired\\n':
        out['verdict']='findings'
        out['findings']=[
            {'id':'wrong-text','priority':'P1','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Sample text is wrong for the stated intent','action':'Rewrite the line to say repaired','rule':'','head':b['Head']},
            {'id':'missing-context','priority':'P2','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Sample text gives no context for readers','action':'Add one line of context','rule':'','head':b['Head']}]
    if b.get('Responses'):
        out['resolutions']=[{'finding':r['finding'],'check':'confirmed','reason':'the line now reads repaired'} for r in b['Responses']]
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'native','model':'default','stopReason':'stop','content':[{'type':'text','text':json.dumps(out)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
'''


def tmux(*args, check=True):
    return subprocess.run(["tmux", "-S", str(socket), "-f", "/dev/null", *args], check=check, capture_output=True, text=True).stdout


class Fixture:
    def __init__(self, name, agents=True):
        self.dir = root / name
        self.repo, self.state, self.bin = self.dir / "repo", self.dir / "state", self.dir / "bin"
        for d in (self.repo, self.bin):
            d.mkdir(parents=True)
        self.state.mkdir(mode=0o700)
        if agents:
            (self.bin / "pi").write_text(fake_pi)
            (self.bin / "claude").write_text("#!/bin/sh\nexit 1\n")
            for name in ("pi", "claude"):
                (self.bin / name).chmod(0o700)
        gate = self.dir / "gate"
        gate.write_text("#!/bin/sh\necho ok\n")
        gate.chmod(0o700)
        for file, value in [("fixer.json", {"version": 2, "executable": str(self.bin / "pi"), "model": "", "name": "Test", "email": "t@example.invalid", "checks": [["/usr/bin/true"]]}),
                            ("pipeline.json", {"version": 2, "budget_unit": "rounds", "name": "Test", "email": "t@example.invalid", "test": [[str(gate), "test"]], "lint": [[str(gate), "lint"]],
                                               "not_applicable": {"docs": "fixture", "push": "local fixture", "pr": "local fixture", "ci": "local fixture"}})]:
            (self.state / file).write_text(json.dumps(value))
            (self.state / file).chmod(0o600)
        self.env = {"HOME": str(self.dir), "PATH": f"{self.bin}:/usr/bin:/bin", "GREENHOUSE_HOME": str(self.state),
                    "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "TMPDIR": os.environ["TMPDIR"], "LANG": "C.UTF-8"}
        for args in [["init", "-b", "main"], ["commit", "--allow-empty", "-m", "base"], ["checkout", "-b", "feature"]]:
            self.git(*args)
        (self.repo / "sample.txt").write_text("candidate\n")
        self.git("add", "sample.txt")
        self.git("commit", "-m", "candidate")

    def session(self, agent, sid, request, files=("sample.txt",), cwd=None):
        cwd = str(cwd or self.repo.resolve())
        stamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 600))
        encoded = re.sub(r"[^A-Za-z0-9]", "-", cwd)
        if agent == "claude":
            path = self.dir / ".claude" / "projects" / encoded / f"{sid}.jsonl"
            records = [{"type": "user", "cwd": cwd, "sessionId": sid, "timestamp": stamp, "message": {"content": request}},
                       {"type": "assistant", "cwd": cwd, "message": {"content": [{"type": "tool_use", "input": {"file_path": f"{cwd}/{f}"}} for f in files]}}]
        else:
            path = self.dir / ".pi" / "agent" / "sessions" / ("--" + encoded.lstrip("-") + "--") / f"{sid}.jsonl"
            records = [{"type": "session", "id": sid, "cwd": cwd, "timestamp": stamp},
                       {"type": "message", "message": {"role": "user", "content": [{"type": "text", "text": request + " in " + " ".join(files)}]}}]
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("".join(json.dumps(r) + "\n" for r in records))

    def git(self, *args):
        subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=t@example.invalid", *args], cwd=self.repo, env=self.env, check=True, capture_output=True)

    def axi(self, op="inspect"):
        p = subprocess.run([binary, "axi"], input=json.dumps({"version": 2, "op": op}), cwd=self.repo, env=self.env, text=True, capture_output=True, check=True, timeout=20)
        return json.loads(p.stdout)

    def settle(self, statuses=("running", "cancelling", "writing"), timeout=60):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            state = self.axi()
            if state["run"]["status"] not in statuses:
                return state
            time.sleep(0.1)
        raise AssertionError(f"run did not settle: {state['run']['status']}")

    def stop_daemon(self):
        pid = self.state / "pid"
        if pid.exists():
            value = json.loads(pid.read_text())
            try:
                cmd = Path(f"/proc/{value}/cmdline").read_bytes().split(b"\0")
            except FileNotFoundError:
                return
            if cmd[:2] == [binary.encode(), b"daemon"]:
                os.kill(value, signal.SIGTERM)
                end = time.monotonic() + 5
                while Path(f"/proc/{value}").exists() and time.monotonic() < end:
                    time.sleep(0.05)


class Session:
    def __init__(self, fixture, width, label, extra=None, height=24):
        self.fixture, self.width, self.height, self.label = fixture, width, height, label
        self.name = f"{label}-{width}"
        env = {**fixture.env, "TERM": "screen-256color", **(extra or {})}
        command = "env -i " + " ".join(f"{k}={v}" for k, v in env.items()) + f" {binary}; echo EXIT=$?; sleep 600"
        tmux("new-session", "-d", "-s", self.name, "-x", str(width), "-y", str(height), "-c", str(fixture.repo), command)
        tmux("set-option", "-t", self.name, "status", "off")
        self.log = evidence / f"walkthrough-{label}-{width}.txt"
        self.log.write_text(f"# {label} at {width}x{height}: keys sent and screens captured from a real PTY\n")
        self.count = 0

    def screen(self, escapes=False):
        args = ["capture-pane", "-p", "-t", self.name]
        if escapes:
            args.append("-e")
        return tmux(*args)

    def keys(self, *keys, literal=False):
        for key in keys:
            if literal:
                tmux("send-keys", "-t", self.name, "-l", key)
            else:
                tmux("send-keys", "-t", self.name, key)
            time.sleep(0.15)
        with self.log.open("a") as f:
            f.write(f"\n$ keys {'type ' if literal else ''}{' '.join(repr(k) for k in keys)}\n")

    def type(self, text):
        self.keys(text, literal=True)

    def wait(self, pattern, timeout=20):
        end = time.monotonic() + timeout
        text = ""
        while time.monotonic() < end:
            text = self.screen()
            if re.search(pattern, text) or re.search(pattern, " ".join(text.split())):
                return text
            time.sleep(0.1)
        raise AssertionError(f"{self.name}: {pattern!r} not on screen:\n{text}")

    def capture(self, step, *expect, absent=()):
        time.sleep(0.3)
        text = self.screen()
        for pattern in expect:
            if not re.search(pattern, text) and not re.search(pattern, " ".join(text.split())):
                text = self.wait(pattern, 10)
        lines = text.rstrip("\n").split("\n")
        assert len(lines) <= self.height, (self.name, step, len(lines))
        for line in lines:
            assert len(line) <= self.width, (self.name, step, line)
        for pattern in absent:
            assert not re.search(pattern, text), (self.name, step, pattern, text)
        self.count += 1
        framed = "\n".join(f"|{line.ljust(self.width)}|" for line in text.split("\n")[:self.height])
        with self.log.open("a") as f:
            f.write(f"\n## {self.count}. {step}\n+{'-' * self.width}+\n{framed}\n+{'-' * self.width}+\n")
        return text

    def resize(self, width):
        tmux("resize-window", "-t", self.name, "-x", str(width), "-y", str(self.height))
        self.width = width
        with self.log.open("a") as f:
            f.write(f"\n$ resize to {width}x{self.height}\n")

    def close(self):
        tmux("kill-session", "-t", self.name, check=False)


def no_agent(width):
    f = Fixture(f"none-{width}", agents=False)
    s = Session(f, width, "no-agent")
    try:
        s.capture("first launch without an installed agent", r"Agent\s+none found: install pi, claude or codex", r"No supported agent found on PATH",
                  absent=[r"pi /", r"Run [0-9a-f]{32}", r"\[i\] Inspect"])
        s.keys("Enter")
        s.capture("Enter does not start anything without an agent", r"Nothing to run from here now")
        state = f.axi()
        assert state["run"]["status"] == "unconfigured" and state["run"]["review_used"] == 0
        s.keys("q")
        s.capture("q quits with a notice", r"Detached\. Nothing was cancelled", r"EXIT=0")
    finally:
        s.close()
        f.stop_daemon()


def first_screen(s):
    return s.capture("first launch: one Run action, nothing collected yet", r"▸ Run review", r"Agent\s+Pi · native default model",
                     r"Intent\s+collected from your agent session on Run", r"Target\s+main \(suggested\)",
                     absent=[r"Run [0-9a-f]{32}", r"\[i\] Inspect"])


def review_started(s):
    s.capture("one Enter collected the intent, prepared, loaded rules and started the review", r"Review started|Independent review · Pi|Review\s+(clean|\d+ outstanding)")
    state = s.fixture.axi()
    assert state["run"]["review_used"] == 1 and state["rules_ready"], state["run"]["status"]
    return state


def cancel_detach(width):
    f = Fixture(f"cancel-{width}")
    s = Session(f, width, "cancel-detach")
    try:
        first_screen(s)
        (f.bin / "delay").write_text("60")
        s.keys("Enter")
        s.capture("no matching agent session: Run asks one question", r"No local agent session started in this worktree", r"What should this change do\?")
        assert f.axi()["run"].get("intent", "") == "" and f.axi()["run"]["review_used"] == 0
        answer = "Fix q and x handling in sample so every reader of sample.txt sees corrected text"
        s.type(answer)
        s.capture("typing past the screen width keeps the cursor end visible; q and x stay in the answer", r"corrected text\s*$|corrected text\s*\n", absent=[r"EXIT="])
        s.keys("BSpace", "BSpace", "BSpace", "BSpace", "BSpace")
        s.capture("deletion is visible", r"corrected\s")
        s.resize(100)
        s.capture("resize while editing keeps the whole answer visible", r"> Fix q and x handling in sample so every reader of sample\.txt sees corrected")
        s.resize(width)
        s.capture("resize back while editing", r"sees corrected")
        s.type(" text")
        s.keys("Enter")
        state = review_started(s)
        assert state["run"]["intent"] == answer, state["run"]["intent"]
        s.capture("live progress: stage, agent and elapsed time", r"Independent review · Pi · \d+s", r"x cancel", r"q detach")
        s.resize(100)
        s.capture("resized to 100 columns while running", r"Independent review · Pi")
        s.resize(width)
        s.capture(f"resized back to {width} columns", r"Independent review · Pi")
        s.keys("x")
        s.capture("one x asks for confirmation", r"Press x again to cancel", r"q only detaches")
        s.keys("Escape")
        assert f.axi()["run"]["status"] == "running"
        s.keys("q")
        s.capture("q detaches and says the review keeps running", r"Detached\. The independent review keeps running", r"EXIT=0")
        assert f.axi()["run"]["status"] == "running"
        s.close()
        s = Session(f, width, "cancel-reattach")
        s.capture("reattach shows the running review", r"Independent review · Pi · ≥\d+s")
        s.keys("x", "x")
        state = f.settle()
        assert state["run"]["status"] == "cancelled", state["run"]["status"]
        s.capture("cancelled after x x; rerun offered", r"Rerun stage", r"cancelled")
        s.keys("q")
    finally:
        s.close()
        f.stop_daemon()


def findings_repair(width):
    f = Fixture(f"repair-{width}")
    s = Session(f, width, "findings-repair")
    try:
        f.session("claude", "11111111-collected", "Change the sample text so it reads correctly")
        first_screen(s)
        s.keys("Tab", "Tab")
        s.capture("Tab moves focus to the Intent row", r"▸○ Intent", r"⏎ edit intent")
        s.keys("Enter")
        s.type("Fix q and x")
        s.capture("editing the intent before Run", r"step 2 of 4", r"Intent\s+> Fix q and x")
        s.keys("Escape")
        s.capture("Esc goes back to the agent step", r"step 1 of 4")
        s.keys("Enter")
        s.capture("the typed intent is kept", r"Intent\s+> Fix q and x")
        s.keys("Enter", "Enter", "Enter")
        s.capture("saved for Run; nothing sent", r"Saved for Run\. Nothing has run yet", r"Intent\s+Fix q and x · typed")
        assert f.axi()["run"]["review_used"] == 0 and f.axi()["run"].get("intent", "") == ""
        (f.bin / "delay").write_text("3")
        s.keys("Escape", "Enter")
        state = review_started(s)
        assert state["run"]["intent"] == "Fix q and x", "the edited intent must win over a collected one"
        f.settle()
        s.capture("findings arrive without a keypress", r"Findings · 2 outstanding findings", r"Open findings", absent=[r"[0-9a-f]{64}"])
        s.keys("Enter")
        s.capture("findings list with selection", r"\[ \] P1 sample.txt:1", r"0 selected")
        s.keys("Enter")
        s.capture("full finding description", r"Action: Rewrite the line to say repaired")
        s.keys("Escape", "Space")
        s.capture("first finding selected", r"\[x\] P1", r"1 selected")
        s.keys("r")
        s.capture("repair asks for confirmation", r"Repair 1 finding", r"uses one round")
        s.keys("Enter")
        state = f.settle()
        assert state["run"]["status"] == "fixed-not-independently-rereviewed", state["run"]["status"]
        assert [x["explanation"] for x in state["run"]["repair"]["selected"]] == ["Sample text is wrong for the stated intent"], state["run"]["repair"]["selected"]
        s.capture("repaired; handback is next", r"Hand back the fix", r"not independently reviewed")
        s.keys("Enter")
        state = f.settle()
        assert state["run"]["repair"]["handback"] == "handed-back", state["run"]["repair"]
        assert (f.repo / "sample.txt").read_text() == "repaired\n"
        s.capture("handed back", r"Fix handed back to your branch", r"Start a new run to review it independently")
        s.keys("i")
        s.capture("details expose IDs, digests and sources", r"Run [0-9a-f]{32}", r"Candidate: [0-9a-f]{40}")
        s.keys("Escape", "?")
        s.capture("help lists the keys", r"Keys", r"q, Ctrl-C\s+detach")
        s.keys("NPage")
        s.capture("help explains unavailable actions without running them", r"not available now")
        s.keys("Escape", "q")
        s.close()
        for name, extra in [("color", {}), ("no-color", {"NO_COLOR": "1"})]:
            s = Session(f, width, name, extra)
            time.sleep(1)
            raw = s.screen(escapes=True)
            (evidence / f"{name}-{width}.ansi").write_text(raw)
            s.capture(f"{name} screen", r"Greenhouse")
            if name == "color":
                assert color.search(raw), "expected color sequences without NO_COLOR"
            else:
                assert not color.search(raw), "NO_COLOR output contains color sequences"
            s.keys("q")
            s.close()
    finally:
        s.close()
        f.stop_daemon()


def settings_decision_reconnect(width):
    f = Fixture(f"decision-{width}")
    s = Session(f, width, "settings-decision")
    try:
        first_screen(s)
        s.keys("s")
        s.capture("settings draft with sources", r"Settings draft", r"Mode\s+auto", r"built-in default")
        s.keys("Down", "Right", "Down", "Enter", "BSpace")
        s.type("1")
        s.keys("Enter")
        s.capture("draft edits are marked and not sent", r"\* Preset\s+full", r"\* Limit\s+1")
        before = f.axi()
        assert before["run"]["configs"][-1]["settings"]["preset"] == "one-pass" and before["defaults"]["preset"] == "one-pass"
        s.keys("Escape")
        s.capture("home keeps the draft", r"Settings draft has changes")
        s.keys("s")
        for _ in range(9):
            s.keys("Down")
        s.capture("apply shows its scope", r"Adds a run-local snapshot")
        s.keys("Enter")
        s.capture("applied to this run", r"Draft applied to this run")
        state = f.axi()
        assert state["run"]["configs"][-1]["settings"]["preset"] == "full" and state["defaults"]["preset"] == "one-pass"
        s.keys("Down")
        s.capture("save shows its scope", r"Changes per-user defaults")
        s.keys("Enter")
        s.capture("saved as default", r"Draft saved for new runs")
        state = f.axi()
        assert state["defaults"]["preset"] == "full" and state["defaults"]["limit"] == 1, state["defaults"]
        s.keys("Escape")
        f.session("claude", "22222222-matching", "Please fix sample.txt so the pipeline passes; keep it one line")
        f.session("claude", "33333333-other-repo", "Unrelated request in another repository", cwd=f.dir / "other-repo")
        s.capture("Run pipeline is the one action", r"▸ Run pipeline")
        s.keys("Enter")
        state = f.settle(("running", "cancelling", "writing", "unconfigured"))
        assert state["run"]["status"] == "needs-decision", state["run"]["status"]
        decision = state["decision"]
        intent = state["run"]["intent"]
        assert intent.startswith("User request collected from Claude session 22222222") and "keep it one line" in intent and "Unrelated" not in intent, intent
        s.capture("decision required", r"Answer decision", r"Decision required \(cap\)")
        s.keys("Enter")
        s.capture("decision options", r"Choose an answer", r"accept-head")
        for _ in range(decision["options"].index("accept-head")):
            s.keys("Down")
        s.keys("Enter")
        s.type("accept this exact head")
        s.capture("decision reason", r"Answer: accept-head", r"accept this exact head")
        s.keys("Enter")
        state = f.settle(("running", "cancelling", "writing", "needs-decision"))
        answer = state["run"]["pipeline"]["decisions"][0]["answer"]
        assert answer["choice"] == "accept-head" and answer["actor"] == "terminal" and answer["reason"] == "accept this exact head", answer
        s.capture("pipeline finished with accepted risk", r"accepted risk")
        f.stop_daemon()
        s.capture("daemon lost: last state kept, reconnecting", r"RECONNECTING", r"accepted risk")
        s.keys("a")
        s.capture("actions menu lists only views while reconnecting", r"only views are listed", absent=[r"Start a new run"])
        s.keys("Escape")
        f.axi()
        s.capture("resynchronized after the daemon returned", r"· live", absent=[r"RECONNECTING"])
        s.keys("q")
    finally:
        s.close()
        f.stop_daemon()


def ambiguous_and_moved(width):
    f = Fixture(f"ambiguous-{width}")
    f.session("claude", "44444444-claude", "Claude asked to fix the sample")
    f.session("pi", "55555555-pi", "Pi asked to reword the sample")
    s = Session(f, width, "ambiguous")
    try:
        first_screen(s)
        s.keys("Enter")
        text = s.capture("two sessions match equally: one question", r"Several agent sessions", r"Claude session 44444444", r"Pi session 55555555", r"Type the intent myself")
        if text.index("Pi session 55555555") > text.index("Claude session 44444444"):
            s.keys("Down")
        s.capture("the Pi session is focused", r"▸ Pi session 55555555")
        s.keys("Enter")
        state = review_started(s)
        assert "Pi session 55555555" in state["run"]["intent"], state["run"]["intent"]
        f.settle()
        s.keys("q")
        s.close()
        g = Fixture(f"moved-{width}")
        g.session("claude", "66666666-moved", "Fix the sample")
        s = Session(g, width, "moved-head")
        first_screen(s)
        g.git("commit", "--allow-empty", "-m", "moved after opening")
        s.keys("Enter")
        s.capture("moved head: Run stops with the controller's reason", r"Run stopped: saving intent and target refused: source head or branch moved", r"nothing was retried")
        state = g.axi()
        assert state["run"]["review_used"] == 0 and state["run"].get("intent", "") == "", state["run"]
        s.keys("q")
        g.stop_daemon()
    finally:
        s.close()
        f.stop_daemon()


def line_mode():
    f = Fixture("line")
    try:
        p = subprocess.run([binary], input="q\n", cwd=f.repo, env=f.env, text=True, capture_output=True, timeout=20)
        assert p.returncode == 0 and "[q] Detach" in p.stdout and "\n> " in p.stdout and "\x1b[?1049h" not in p.stdout, p.stdout
        (evidence / "line-mode-piped.txt").write_text("\n".join(line.rstrip() for line in p.stdout.replace("\x1b[2J\x1b[H", "").splitlines()) + "\n")
        s = Session(f, 80, "term-dumb", {"TERM": "dumb"})
        s.wait(r"> $|>\s*$")
        s.capture("TERM=dumb keeps the line client", r"\[q\] Detach", r"\[i\] Inspect")
        s.keys("q", "Enter")
        s.capture("line client detaches on q", r"EXIT=0")
        s.close()
    finally:
        f.stop_daemon()


try:
    tmux("new-session", "-d", "-s", "server", "sleep 3600")
    tmux("set-option", "-s", "escape-time", "0")
    for width in (56, 80, 100):
        no_agent(width)
    cancel_detach(56)
    findings_repair(80)
    settings_decision_reconnect(100)
    ambiguous_and_moved(100)
    line_mode()
    print(f"PASS: TUI at 56, 80 and 100 columns by 24 rows: missing agent, one Run collecting intent from a synthetic agent session with history isolation, no-match question, ambiguity choice, edited intent with Esc, moved head, live progress, resize while running, cancel versus detach and reattach, findings selection, repair and handback, details and help, settings draft with Apply and Save, decision, daemon loss and resync, NO_COLOR, piped stdin and TERM=dumb line mode; fixture {root}")
finally:
    tmux("kill-server", check=False)
