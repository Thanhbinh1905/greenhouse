import fcntl
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
import time

binary = str(Path(sys.argv[1]).resolve())
root = Path(tempfile.mkdtemp(prefix="gh-pty-"))
repo = root / "repo"
repo.mkdir()
env = dict(os.environ, HOME=str(root), GREENHOUSE_HOME=str(root / "state"),
           GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL="/dev/null")
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
import json,sys
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
    for width in [56, 100]:
        session(width, ["n", "new", "p", "Inspect committed sample", target, "r", "c", "v", "q"], "-dispatch")
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline and axi()["run"]["status"] == "running":
            time.sleep(0.02)
        reviewed = axi()
        assert reviewed["run"]["status"] == "outstanding-findings", reviewed
        findings = session(width, ["i", "v", "q"], "-findings")
        assert "Fixture finding" in findings and "one-pass" in findings
        assert axi()["run"]["review_used"] == 1
    print(f"PASS: 56- and 100-column PTY Settings, Apply, Save, New run, rule resolution, criteria, load failures, detach, and AXI parity; fixture {root}")
finally:
    pidfile = root / "state" / "pid"
    if pidfile.exists():
        os.kill(json.loads(pidfile.read_text()), signal.SIGTERM)
