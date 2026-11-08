"""Drive live events and a host-owned run in real PTYs at 56, 80 and 100 columns.

usage: events_check.py BINARY EVIDENCE_DIR [before]

With "before", the script records what an older binary shows for the same
steps instead of asserting the new behavior.
"""
import fcntl
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

binary = str(Path(sys.argv[1]).resolve())
evidence = Path(sys.argv[2])
before = len(sys.argv) > 3 and sys.argv[3] == "before"
prefix = "before" if before else "after"
host = {"name": "fake-host", "id": "task-7"}


class ModelHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"data": [{"id": "model"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


provider = HTTPServer(("127.0.0.1", 0), ModelHandler)
threading.Thread(target=provider.serve_forever, daemon=True).start()


def git(cwd, env, *args):
    return subprocess.run(["git", *args], cwd=cwd, env=env, check=True, capture_output=True, text=True).stdout.strip()


def fixture(width):
    root = Path(tempfile.mkdtemp(prefix=f"gh-events-{width}-", dir=os.environ["TMPDIR"]))
    state = root / "state"
    state.mkdir(mode=0o700)
    env = dict(os.environ, HOME=str(root), GREENHOUSE_HOME=str(state), GIT_CONFIG_NOSYSTEM="1",
               GIT_CONFIG_GLOBAL="/dev/null", GREENHOUSE_TEST_PROVIDER_KEY="fixture-secret")
    fake = root / "fake-pi"
    fake.write_text('''#!/usr/bin/python3
import json,sys,time
if '--list-models' in sys.argv:
 print('provider model')
 print('fixture model')
 sys.exit(0)
b=json.load(sys.stdin)
time.sleep(1)
f={"id":"fixture","priority":"P2","origin":"correctness","file":"sample.txt","start":1,"end":1,"explanation":"Fixture finding","action":"Inspect fixture","rule":"","head":b["Head"]}
v={"head":b["Head"],"verdict":"findings","coverage":b["Paths"],"findings":[f]}
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
''')
    fake.chmod(0o700)
    (state / "reviewer.json").write_text(json.dumps({"version": 2, "executable": str(fake), "model": "fixture/model"}))
    (state / "providers.json").write_text(json.dumps({"version": 2, "providers": {"fixture": {
        "base_url": f"http://127.0.0.1:{provider.server_port}", "api_key_env": "GREENHOUSE_TEST_PROVIDER_KEY", "api": "openai-completions"}}, "models": []}))
    for name in ["reviewer.json", "providers.json"]:
        os.chmod(state / name, 0o600)
    repos = []
    for name in ["standalone", "hosted"]:
        repo = root / name
        repo.mkdir()
        git(repo, env, "init", "-b", "main")
        git(repo, env, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
        target = git(repo, env, "rev-parse", "HEAD")
        (repo / "sample.txt").write_text("candidate\n")
        git(repo, env, "add", "sample.txt")
        git(repo, env, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
        repos.append((repo, target))
    return env, repos


def axi(env, repo, request):
    result = subprocess.run([binary, "axi"], input=json.dumps(request), text=True, capture_output=True, env=env, cwd=repo)
    return json.loads(result.stdout)


def step(env, repo, v, op, extra=None, version=1):
    q = {"version": version, "op": op, "run_id": v.get("run_id"), "input_id": v.get("input_id"), "gate_id": v.get("gate_id"),
         "expected": v.get("token"), "action_id": f"{op}-{time.monotonic_ns()}", "reason": "events check"}
    q.update(extra or {})
    return axi(env, repo, q)


def ready(env, repo, target, extra=None, version=1):
    attach = {"version": version, "op": "attach"}
    attach.update(extra or {})
    v = axi(env, repo, attach)
    settings = {"mode": "auto", "preset": "one-pass", "limit": 4, "rules": ["unslop"], "reviewer_backend": "pi",
                "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
    for op, more in [("apply", {"settings": settings}), ("prepare", {"intent": "Review sample change", "target": target}), ("resolve-rules", {})]:
        more.update(extra or {})
        v = step(env, repo, v, op, more, version)
        if v["outcome"] != "ok":
            return v
    return v


class Terminal:
    def __init__(self, env, repo, width):
        self.width = width
        self.master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, width, 0, 0))
        self.child = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env, cwd=repo, start_new_session=True)
        os.close(slave)
        self.captured = bytearray()

    def text(self):
        return self.captured.decode(errors="replace").replace("\r", "")

    def wait_for(self, needle, seconds=15):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if needle in " ".join(self.text().split()):
                return True
            ready, _, _ = select.select([self.master], [], [], 0.2)
            if ready:
                try:
                    self.captured.extend(os.read(self.master, 65536))
                except OSError:
                    break
        return needle in " ".join(self.text().split())

    def drain(self, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            ready, _, _ = select.select([self.master], [], [], 0.05)
            if ready:
                try:
                    self.captured.extend(os.read(self.master, 65536))
                except OSError:
                    return

    def type(self, line):
        os.write(self.master, line.encode() + b"\n")

    def close(self, name):
        self.type("q")
        self.child.wait(timeout=10)
        self.drain(0.2)
        os.close(self.master)
        text = self.text()
        destination = evidence / f"{prefix}-{self.width}-{name}.log"
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text("\n".join(line.rstrip() for line in text.splitlines()) + "\n")
        for frame in text.split("\x1b[2J\x1b[H")[1:]:
            home = frame.split("\n> ")[0]
            assert all(len(line) <= self.width or line.split() == [line] for line in home.splitlines()), (self.width, home)
        return text, self.child.returncode


def check(width):
    env, [(standalone, target), (hosted, host_target)] = fixture(width)
    v = ready(env, standalone, target)
    term = Terminal(env, standalone, width)
    term.wait_for("> ")
    if not before:
        assert term.wait_for("Events: live"), term.text()
    frames = term.text().count("\x1b[2J")
    started = step(env, standalone, v, "review")
    assert started["outcome"] == "ok", started
    followed = term.wait_for("outstanding-findings", 20 if not before else 5)
    redraws = term.text().count("\x1b[2J") - frames
    text, code = term.close("events")
    print(f"{width} columns: review followed without input={followed}, redraws={redraws}")
    if not before:
        assert followed and redraws >= 2 and code == 0, text
        assert "Stage: independent-review | running" in text and "Fixture finding" in text

    hosted_v = ready(env, hosted, host_target, {"host": host}, 2)
    if hosted_v["outcome"] != "ok":
        print(f"{width} columns: host attach refused: {hosted_v.get('error')}")
        assert before, hosted_v
        return
    term = Terminal(env, hosted, width)
    term.wait_for("Events: live")
    review = step(env, hosted, hosted_v, "review", {"host": host, "grant": "round-1"}, 2)
    assert review["outcome"] == "ok", review
    assert term.wait_for("grant round-1 | findings | current true", 20), term.text()
    term.type("v")
    assert term.wait_for("refused run is owned by host fake-host task-7"), term.text()
    text, code = term.close("host")
    assert code == 0 and "owned by host fake-host" in text, text
    print(f"{width} columns: host run followed and terminal mutation refused")


for width in [56, 80, 100]:
    check(width)
