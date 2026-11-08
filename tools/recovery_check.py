"""Drive the terminal recovery screen in real PTYs at 56, 80 and 100 columns.

Each width gets its own repository and state home. A Git wrapper kills the
daemon right after the selected repair commits, so restart parks the repair
as an unknown writer outcome. The terminal then captures the screen before
recovery, answers the recovery with `e`, and captures the screen after it:
adopt at 56 columns, discard at 80 and new-run at 100.

Usage: python3 tools/recovery_check.py <greenhouse binary> <evidence dir>
"""
import fcntl
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import pty
import select
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

binary = str(Path(sys.argv[1]).resolve())
evidence = Path(sys.argv[2])


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

REVIEWER = '''#!/usr/bin/python3
import json,sys
if '--list-models' in sys.argv:
 print('provider model')
 print('fixture model')
 sys.exit(0)
b=json.load(sys.stdin)
f={"id":"fixture","priority":"P2","origin":"correctness","file":"sample.txt","start":1,"end":1,"explanation":"Fixture finding","action":"Repair the sample","rule":"","head":b["Head"]}
v={"head":b["Head"],"verdict":"findings","coverage":b["Paths"],"findings":[f]}
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
'''
FIXER = '''#!/usr/bin/python3
import json,sys
json.load(sys.stdin)
with open("sample.txt", "w") as f: f.write("repaired\\n")
'''
GIT_WRAPPER = '''#!/usr/bin/python3
import json,os,subprocess,sys
args=sys.argv[1:]
p=subprocess.run([REAL]+args)
if p.returncode == 0 and 'commit' in args and any('/candidates/' in a for a in args) and os.path.exists(MARKER):
    os.unlink(MARKER)
    home=os.environ['GREENHOUSE_HOME']
    os.kill(json.load(open(os.path.join(home,'pid'))),9)
sys.exit(p.returncode)
'''


def fixture(width):
    root = Path(tempfile.mkdtemp(prefix=f"gh-recovery-{width}-", dir=os.environ["TMPDIR"]))
    repo, state, bindir = root / "repo", root / "state", root / "bin"
    for d in (repo, bindir):
        d.mkdir()
    state.mkdir(mode=0o700)
    marker = root / "kill-after-commit"
    marker.write_text("1")
    wrapper = bindir / "git"
    wrapper.write_text(GIT_WRAPPER.replace("REAL", repr(shutil.which("git"))).replace("MARKER", repr(str(marker))))
    wrapper.chmod(0o700)
    env = dict(os.environ, HOME=str(root), GREENHOUSE_HOME=str(state), PATH=f"{bindir}:{os.environ['PATH']}",
               GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL="/dev/null", GREENHOUSE_TEST_PROVIDER_KEY="fixture-secret")
    for name, body in [("fake-pi", REVIEWER), ("fake-fixer", FIXER)]:
        (root / name).write_text(body)
        (root / name).chmod(0o700)
    configs = {
        "providers.json": {"providers": {"fixture": {"base_url": f"http://127.0.0.1:{provider.server_port}", "api_key_env": "GREENHOUSE_TEST_PROVIDER_KEY", "api": "openai-completions"}}, "models": []},
        "reviewer.json": {"executable": str(root / "fake-pi"), "model": "fixture/model"},
        "fixer.json": {"executable": str(root / "fake-fixer"), "model": "fixture/model", "name": "Test", "email": "test@example.invalid",
                       "checks": [["/usr/bin/python3", "-c", "from pathlib import Path; assert Path('sample.txt').read_text() == 'repaired\\n'"]]},
    }
    for name, value in configs.items():
        (state / name).write_text(json.dumps(value))
        (state / name).chmod(0o600)
    git = lambda *args: subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", *args], cwd=repo, env=env, check=True, capture_output=True, text=True).stdout.strip()
    git("init", "-b", "main")
    git("commit", "--allow-empty", "-m", "initial")
    target = git("rev-parse", "HEAD")
    (repo / "sample.txt").write_text("review me\n")
    git("add", "sample.txt")
    git("commit", "-m", "candidate")
    return root, repo, env, target


def axi(env, repo, request):
    result = subprocess.run([binary, "axi"], input=json.dumps(request), text=True, capture_output=True, env=env, cwd=repo, check=True)
    return json.loads(result.stdout)


def mutate(env, repo, view, op, **fields):
    request = {"version": 1, "op": op, "run_id": view["run_id"], "input_id": view["input_id"], "gate_id": view["gate_id"],
               "expected": view["token"], "action_id": f"{op}-{time.monotonic_ns()}", "reason": "recovery check"}
    request.update(fields)
    response = axi(env, repo, request)
    assert response["outcome"] == "ok", response
    return response


def wait(env, repo, done, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            view = axi(env, repo, {"version": 1, "op": "inspect"})
        except subprocess.CalledProcessError:
            time.sleep(0.05)
            continue
        if done(view):
            return view
        time.sleep(0.05)
    raise AssertionError(f"condition not reached: {view['run']['status']}")


def session(env, repo, width, commands, label):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, width, 0, 0))
    child = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env, cwd=repo, start_new_session=True)
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
    evidence.mkdir(parents=True, exist_ok=True)
    (evidence / f"terminal-{width}-recovery-{label}.log").write_text("\n".join(line.rstrip() for line in text.splitlines()) + "\n")
    for frame in text.split("\x1b[2J\x1b[H")[1:]:
        home = frame.split("\n> ")[0]
        assert all(len(line) <= width for line in home.splitlines()), (width, label, home)
    return text


def scenario(width, choice):
    root, repo, env, target = fixture(width)
    try:
        view = axi(env, repo, {"version": 1, "op": "attach"})
        view = mutate(env, repo, view, "apply", settings={"mode": "auto", "preset": "one-pass", "limit": 4, "rules": ["unslop"], "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
        view = mutate(env, repo, view, "prepare", intent="Repair the sample", target=target)
        view = mutate(env, repo, view, "resolve-rules")
        mutate(env, repo, view, "review")
        view = wait(env, repo, lambda v: v["run"]["status"] == "outstanding-findings")
        mutate(env, repo, view, "repair", selected=[view["run"]["findings"][0]["id"]])
        deadline = time.monotonic() + 20
        while (root / "kill-after-commit").exists() and time.monotonic() < deadline:
            time.sleep(0.05)
        parked = wait(env, repo, lambda v: (v["run"].get("recovery") or {}).get("probed_at") and len(v["run"]["recovery"]["options"]) > 1)
        assert parked["run"]["status"] == "recovery-required", parked["run"]["status"]
        assert parked["run"]["recovery"]["options"] == ["inspect", "adopt", "discard", "new-run"], parked["run"]["recovery"]
        before = session(env, repo, width, ["i", "q"], "before")
        assert "RECOVERY REQUIRED: repair at selected-repair" in before and "Options: inspect, adopt, discard, new-run" in before
        session(env, repo, width, ["e", "inspect", "check the evidence", "e", "keep", "e", choice, f"{choice} after the controller crash", "q"], "action")
        settled = wait(env, repo, lambda v: v["run"]["status"] not in ("writing", "recovery-required"))
        after = session(env, repo, width, ["i", "q"], "after")
        if choice == "new-run":
            predecessor = settled["run"]["predecessor"]
            assert f"Linked from run {predecessor}" in after, after
            old = axi(env, repo, {"version": 1, "op": "inspect", "run_id": predecessor})["run"]
            assert old["status"] == "superseded" and old["recoveries"][0]["answer"]["actor"] == "terminal", old
        else:
            assert f"Recovered repair: {choice} by terminal" in after, after
        run = settled["run"]
        if choice == "adopt":
            assert run["status"] == "fixed-not-independently-rereviewed" and run["repair_used"] == 1, run
        elif choice == "discard":
            assert run["status"] == "fixed-not-independently-rereviewed" and run["repair_used"] == 2 and run["prior_repairs"][0]["status"] == "recovery-discarded", run
        else:
            assert run["predecessor"] and run["repair_used"] == 1 and run["status"] == "unconfigured", run
        answered = [e for e in (run.get("events") or []) if e["kind"] in ("recovery-adopt", "recovery-discard")]
        assert choice == "new-run" or answered, run["events"]
        assert "unknown choice" in (evidence / f"terminal-{width}-recovery-action.log").read_text()
    finally:
        try:
            pid = json.loads((root / "state" / "pid").read_text())
            os.kill(pid, 15)
        except (OSError, ValueError):
            pass


for width, choice in [(56, "adopt"), (80, "discard"), (100, "new-run")]:
    scenario(width, choice)
print("PASS: recovery screen before and after adopt (56), discard (80) and new-run (100) in real PTYs")
