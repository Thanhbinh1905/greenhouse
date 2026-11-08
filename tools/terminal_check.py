import fcntl
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

binary = str(Path(sys.argv[1]).resolve())
root = Path(tempfile.mkdtemp(prefix="gh-pty-", dir=os.environ["TMPDIR"]))
repo = root / "repo"
repo.mkdir()
env = dict(os.environ, HOME=str(root), GREENHOUSE_HOME=str(root / "state"),
           GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL="/dev/null")
delivery = {"bare": None, "head": None}


class ForgeHandler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

    def send_json(self, value):
        payload = json.dumps(value).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/repos/acme/garden/pulls":
            self.send_json([])
        elif path == "/repos/acme/garden/pulls/12":
            self.send_json({"number": 12, "head": {"sha": delivery["head"]}})
        elif path.endswith("/check-runs"):
            sha = path.split("/")[-2]
            self.send_json({"total_count": 1, "check_runs": [{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha}]})
        else:
            self.send_error(404)

    def do_POST(self):
        if self.path != "/repos/acme/garden/pulls":
            self.send_error(404)
            return
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        branch = request["head"].removeprefix("acme:")
        delivery["head"] = subprocess.check_output(["git", "--git-dir", delivery["bare"], "rev-parse", f"refs/heads/{branch}"], text=True).strip()
        self.send_json({"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": {"sha": delivery["head"]}})


forge = ThreadingHTTPServer(("127.0.0.1", 0), ForgeHandler)
threading.Thread(target=forge.serve_forever, daemon=True).start()
env["GREENHOUSE_GITHUB_API"] = f"http://127.0.0.1:{forge.server_port}"
for args in [["init", "-b", "main"], ["-c", "user.name=Test", "-c",
             "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"]]:
    subprocess.run(["git", *args], cwd=repo, env=env, check=True, capture_output=True)


def axi():
    result = subprocess.run([binary, "axi"], input='{"version":1,"op":"attach"}',
                            text=True, capture_output=True, env=env, cwd=repo, check=True)
    return json.loads(result.stdout)


def session(width, commands, label=""):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 28, width, 0, 0))
    child = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave,
                             env=env, cwd=repo, start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def until_prompt():
        chunk = bytearray()
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            ready, _, _ = select.select([master], [], [], 0.5)
            if ready:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    break
                chunk.extend(data)
                if chunk.endswith(b"> "):
                    captured.extend(chunk)
                    return
        raise AssertionError(f"terminal prompt missing: {chunk!r}")

    try:
        until_prompt()
        for command in commands:
            if command == ":commit":
                subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                                "commit", "--allow-empty", "-m", "new source head"],
                               cwd=repo, env=env, check=True, capture_output=True)
                continue
            os.write(master, command.encode() + b"\n")
            if command != "q":
                until_prompt()
        child.wait(timeout=10)
        assert child.returncode == 0
    finally:
        if child.poll() is None:
            child.kill()
            child.wait()
        os.close(master)
    text = captured.decode().replace("\r", "")
    destination = Path(sys.argv[2]) / f"terminal-{width}{label}.log"
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text("\n".join(line.rstrip() for line in text.splitlines()) + "\n")
    frames = text.split("\x1b[2J\x1b[H")[1:]
    for frame in frames:
        home = frame.split("\n> ")[0]
        assert all(len(line) <= width for line in home.splitlines()), (width, home)
    return text


try:
    narrow = session(56, ["s", "ask one-pass 7", "unslop", "a", "q"])
    state = axi()
    assert state["run"]["configs"][-1]["settings"]["mode"] == "ask"
    assert state["defaults"]["mode"] == "auto"
    assert state["run"]["configs"][-1]["settings"]["rules"] == ["unslop"]
    wide = session(100, ["i", "s", "ask one-pass 7", "unslop", "d", ":commit", "i", "n", "new", "q"])
    next_state = axi()
    assert state["run_id"] != next_state["run_id"]
    assert state["run"]["candidate"] != next_state["run"]["candidate"]
    assert next_state["run"]["configs"][0]["settings"]["mode"] == "ask"
    assert "Mode: ask" in wide
    session(56, ["q"], "-reattach")
    for width in [56, 100]:
        rules = session(width, ["r", "c", "q"], "-rules")
        resolved = axi()
        assert resolved["rules_ready"]
        assert resolved["rules"][0]["status"] == "resolved"
        assert "Digest:" in rules and "[advisory]" in rules
        assert "Flag inflated prose" in rules
        failures = session(width, ["s", "ask one-pass 7", "missing,?optional", "a", "r", "c", "q"], "-failures")
        failed = axi()
        assert not failed["rules_ready"]
        assert all(rule["status"] == "failed" for rule in failed["rules"])
        assert "Load error:" in failures
        session(width, ["s", "ask one-pass 7", "unslop", "a", "q"], "-restore")
    assert axi()["run_id"] == next_state["run_id"]
    target = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, env=env, text=True).strip()
    (repo / "sample.txt").write_text("candidate\n")
    for args in [["add", "sample.txt"], ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate"]]:
        subprocess.run(["git", *args], cwd=repo, env=env, check=True, capture_output=True)
    fake = root / "fake-pi"
    fake.write_text('''#!/usr/bin/python3
import json,sys,os
with open(os.path.join(os.path.dirname(__file__), "review-calls"), "a") as calls: calls.write("review\\n")
b=json.load(sys.stdin)
f={"id":"fixture","priority":"P2","origin":"correctness","file":"sample.txt","start":1,"end":1,"explanation":"Fixture finding","action":"Inspect fixture","rule":"","head":b["Head"]}
v={"head":b["Head"],"verdict":"findings","coverage":b["Paths"],"findings":[f]}
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
''')
    fake.chmod(0o700)
    config = root / "state" / "reviewer.json"
    config.write_text(json.dumps({"executable":str(fake),"model":"fixture/model"}))
    config.chmod(0o600)
    fixer = root / "fake-fixer"
    fixer.write_text('''#!/usr/bin/python3
import json,sys
b=json.load(sys.stdin)
with open("sample.txt", "w") as f: f.write("repaired\\n")
''')
    fixer.chmod(0o700)
    fixer_config = root / "state" / "fixer.json"
    fixer_config.write_text(json.dumps({"executable":str(fixer),"model":"fixture/model","name":"Test","email":"test@example.invalid","checks":[["/usr/bin/python3","-c","from pathlib import Path; assert Path('sample.txt').read_text() == 'repaired\\n'"]] }))
    fixer_config.chmod(0o600)
    session(56, ["n", "new", "p", "Inspect committed sample", target, "r", "c", "v", "q"], "-dispatch")
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline and axi()["run"]["status"] == "running":
        time.sleep(0.02)
    reviewed = axi()
    assert reviewed["run"]["status"] == "outstanding-findings", reviewed
    findings = session(56, ["i", "q"], "-findings")
    assert "Fixture finding" in findings and "one-pass" in findings
    assert reviewed["run"]["review_used"] == 1
    finding_id = reviewed["run"]["findings"][0]["id"]
    (repo / "dirty-user-file").write_text("preserve")
    repair_flow = session(56, ["f", finding_id, "i", "q"], "-repair")
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline and axi()["run"]["status"] == "writing": time.sleep(0.02)
    fixed = axi()
    assert fixed["run"]["status"] == "fixed-not-independently-rereviewed", fixed
    repair = fixed["run"]["repair"]
    assert repair["checks"][0]["status"] == "passed" and repair["head"]
    repair_result = session(100, ["i", "q"], "-repair-result")
    assert "Check" in repair_result and "fixed-not-independently-rereviewed" in repair_result
    refused = session(56, ["h", "q"], "-handback-refused")
    blocked = axi()
    assert blocked["run"]["repair"]["handback"] == "awaiting-handback"
    assert "dirty state" in " ".join(blocked["blocked"]) or "dirty state" in refused
    refused_wide = session(100, ["h", "q"], "-handback-refused-wide")
    assert "dirty state" in refused_wide
    assert (repo / "dirty-user-file").read_text() == "preserve"
    (repo / "dirty-user-file").unlink()
    handed = session(100, ["h", "q"], "-handback")
    final = axi()
    assert final["run"]["repair"]["handback"] == "handed-back"
    assert "handed-back" in handed
    assert (root / "review-calls").read_text().count("review") == 1
    full_reviewer = root / "full-pi"
    full_reviewer.write_text('''#!/usr/bin/python3
import json,sys,os
root=os.path.dirname(__file__)
b=json.load(sys.stdin)
mode=open(root+"/full-review-mode").read() if os.path.exists(root+"/full-review-mode") else "clean"
v={"head":b["Head"],"verdict":"clean","coverage":b["Paths"],"findings":[],"documentation":{"status":"not-needed","reason":"fixture changes no public behavior"}}
if mode=="findings":
    v["verdict"]="findings"
    v["findings"]=[{"id":"x","priority":"P1","origin":"correctness","file":"sample.txt","start":1,"end":1,"explanation":"Repaired sample still wrong","action":"Fix it","rule":"","head":b["Head"]}]
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
''')
    full_reviewer.chmod(0o700)
    config.write_text(json.dumps({"executable": str(full_reviewer), "model": "fixture/model"}))
    gate = root / "gate"
    gate.write_text('''#!/usr/bin/python3
import sys
name=sys.argv[1]
got=open("sample.txt").read()
if name=="test" and got!="repaired\\n":
    print("FAIL test: sample.txt is "+repr(got))
    sys.exit(1)
print("ok "+name)
''')
    gate.chmod(0o700)
    full_fixer = root / "full-fixer"
    full_fixer.write_text('''#!/usr/bin/python3
import json,sys,os
root=os.path.dirname(__file__)
json.load(sys.stdin)
if os.path.exists(root+"/fix-then-review"):
    open(root+"/full-review-mode","w").write("findings")
open("sample.txt","w").write("repaired\\n")
''')
    full_fixer.chmod(0o700)
    fixer_config.write_text(json.dumps({"executable": str(full_fixer), "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [["/usr/bin/true"]]}))

    def full_repo(name, plan):
        global repo
        repo = root / name
        repo.mkdir()
        for args in [["init", "-b", "main"], ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"]]:
            subprocess.run(["git", *args], cwd=repo, env=env, check=True, capture_output=True)
        base = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, env=env, text=True).strip()
        bare = root / f"{name}.git"
        subprocess.run(["git", "init", "--bare", str(bare)], env=env, check=True, capture_output=True)
        delivery["bare"] = str(bare)
        (repo / "sample.txt").write_text("candidate\n")
        for args in [["add", "sample.txt"], ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate"]]:
            subprocess.run(["git", *args], cwd=repo, env=env, check=True, capture_output=True)
        plan_file = root / "state" / "pipeline.json"
        plan_file.write_text(json.dumps(dict(plan, name="Test", email="test@example.invalid", remote=str(bare), github_owner="acme", github_repo="garden", pull_base="main", required_checks=["fixture-ci"])))
        plan_file.chmod(0o600)
        return base

    def settle():
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline and axi()["run"]["status"] in ("running", "cancelling"):
            time.sleep(0.05)
        return axi()

    markers = {"passed": "[pass]", "repaired-unreviewed": "[REPAIRED, NOT REVIEWED]", "failed": "[FAIL]",
               "not-applicable": "[n/a]", "needs-decision": "[DECISION]", "pending": "[ ]", "unsupported": "[unsupported]", "unconfigured": "[UNCONFIGURED]"}

    def parity(state, text):
        for stage in state["run"]["pipeline"]["stages"]:
            marker = markers[stage["status"]]
            if stage["status"] == "passed" and stage["repairs"]:
                marker = "[pass after repair, rereviewed]"
            assert f"{marker} {stage['name']}\n" in text, stage
        assert f"Pipeline: {state['run']['pipeline']['status']}" in text

    commands = {"test": [[str(gate), "test"]], "lint": [[str(gate), "lint"]], "not_applicable": {"docs": "fixture publishes no documentation"}}
    widths = {"full-ready": 56, "full-failed": 100, "full-unreviewed": 56}
    for name, extra in [("full-ready", {"repairs": ["test"], "repair_limit": 1}), ("full-failed", {}), ("full-unreviewed", {"repairs": ["test"], "repair_limit": 1})]:
        (root / "full-review-mode").unlink(missing_ok=True)
        (root / "fix-then-review").unlink(missing_ok=True)
        if name == "full-unreviewed":
            (root / "fix-then-review").write_text("1")
        target = full_repo(name, dict(commands, **extra))
        width = widths[name]
        session(width, ["s", "auto full 6", "unslop", "a", "p", "Change the sample", target, "g", "q"], "-" + name)
        state = settle()
        other = 100 if width == 56 else 56
        text = session(width, ["i", "q"], f"-{name}-result")
        delivery_text = session(other, ["i", "c", "q"], f"-{name}-result-wide" if other == 100 else f"-{name}-result-narrow")
        parity(state, text)
        stages = {s["name"]: s for s in state["run"]["pipeline"]["stages"]}
        if name == "full-ready":
            assert state["run"]["status"] == "ci-ready", state["run"]["pipeline"]
            assert stages["push"]["status"] == "passed" and stages["pr"]["status"] == "passed" and stages["ci"]["status"] == "passed"
            assert stages["pr"]["url"] == "https://example.invalid/acme/garden/pull/12"
            assert stages["ci"]["head"] == state["run"]["candidate"]
            assert "Pipeline: ci-ready" in text and "https://example.invalid/acme/garden/pull/12" in text
            assert "Pipeline: ci-ready" in delivery_text and "https://example.invalid/acme/garden/pull/12" in delivery_text
            assert stages["test"]["repairs"] == 1 and stages["test"]["status"] == "passed"
            assert "[pass after repair, rereviewed] test" in text and "Gate repairs: 1/1" in text
            assert "[pass] review" in text and "[n/a] docs" in text
        if name == "full-failed":
            assert state["run"]["status"] == "needs-decision" and stages["test"]["status"] == "failed"
            assert "[FAIL] test" in text and "| FAIL test" in text and "stopped at test" in text
            assert "[ ] lint" in text and "Gate repairs: 0/0" in text
        if name == "full-unreviewed":
            assert stages["test"]["status"] == "repaired-unreviewed", stages
            assert "[REPAIRED, NOT REVIEWED] test" in text and "[DECISION] review" in text
    forge.shutdown()
    print(f"PASS: 56- and 100-column PTY selected repair, check, dirty handback refusal, safe handback, full delivery to CI-ready, pipeline repair, failure diagnostics and AXI parity; fixture {root}")
finally:
    pidfile = root / "state" / "pid"
    if pidfile.exists():
        os.kill(json.loads(pidfile.read_text()), signal.SIGTERM)
