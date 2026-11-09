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
evidence = Path(sys.argv[2])
evidence.mkdir(parents=True, exist_ok=True)

for width in (56, 80, 100):
    root = Path(tempfile.mkdtemp(prefix=f"native-{width}-", dir=os.environ["TMPDIR"]))
    repo, agents, state = root / "repo", root / "bin", root / "state"
    repo.mkdir()
    agents.mkdir()
    agent = agents / "pi"
    agent.write_text('''#!/usr/bin/python3
import json,os,sys
assert os.sched_getaffinity(0)==os.sched_getaffinity(os.getppid())
assert '--no-extensions' in sys.argv
assert '--no-mcp' in sys.argv and '--no-context-files' in sys.argv
assert '--model' not in sys.argv
if '--list-models' in sys.argv:
 print('provider model\\nnative default')
 sys.exit(0)
assert '--no-session' in sys.argv
b=json.load(sys.stdin)
v={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'native','model':'default','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
''')
    agent.chmod(0o700)
    env = dict(os.environ, HOME=str(root), GREENHOUSE_HOME=str(state), PATH=f"{agents}:/usr/bin:/bin",
               GIT_CONFIG_GLOBAL="/dev/null", GIT_CONFIG_NOSYSTEM="1")

    def git(*args):
        return subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=t@example.invalid", *args],
                              cwd=repo, env=env, check=True, capture_output=True, text=True).stdout.strip()

    git("init", "-b", "main")
    git("commit", "--allow-empty", "-m", "base")
    git("checkout", "-b", "feature")
    (repo / "sample.txt").write_text("review me\n")
    git("add", ".")
    git("commit", "-m", "change")
    (repo / "dirty.txt").write_text("preserve this\n")

    def inspect():
        p = subprocess.run([binary, "axi"], input='{"version":2,"op":"inspect"}', cwd=repo,
                           env=env, text=True, capture_output=True, check=True, timeout=10)
        return json.loads(p.stdout)

    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 28, width, 0, 0))
    child = subprocess.Popen([binary], cwd=repo, env=env, stdin=slave, stdout=slave, stderr=slave)
    os.close(slave)
    captured = bytearray()

    def prompt():
        end = time.monotonic() + 10
        chunk = bytearray()
        while time.monotonic() < end:
            if select.select([master], [], [], 0.1)[0]:
                data = os.read(master, 65536)
                chunk.extend(data)
                if chunk.endswith(b"> "):
                    captured.extend(chunk)
                    return
        raise AssertionError(f"missing prompt: {chunk!r}")

    try:
        prompt()
        for command in ["p", "", "Add sample text", "", "l", "v"]:
            os.write(master, (command + "\n").encode())
            prompt()
        end = time.monotonic() + 15
        while time.monotonic() < end and inspect()["run"]["status"] == "running":
            time.sleep(0.05)
        result = inspect()
        assert result["run"]["status"] == "clean-review", result
        for command in ["i", "s", "ask one-pass 4", "", "", "", "", "unslop,no-comments", "", "a"]:
            os.write(master, (command + "\n").encode())
            prompt()
        os.write(master, b"q\n")
        child.wait(timeout=5)
        text = captured.decode()
        assert "Installed agents: pi" in " ".join(text.split()), text
        assert "target [main]" in " ".join(text.split()), text
        assert "Current values and sources:" in text, text
        assert (repo / "dirty.txt").read_text() == "preserve this\n"
        assert not any((state / name).exists() for name in ("reviewer.json", "providers.json", "backends.json", "fixer.json"))
        (evidence / f"native-{width}.log").write_text(text)
        print(f"PASS: fresh review and Settings at {width} columns")
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=5)
        os.close(master)
        if (state / "pid").exists():
            os.kill(json.loads((state / "pid").read_text()), signal.SIGTERM)
