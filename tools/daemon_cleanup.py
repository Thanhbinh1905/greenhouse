import os
from pathlib import Path
import signal
import time


def _proc_info(pid):
    try:
        fields = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
    except OSError:
        return None
    return fields[0], fields[19]


def _same_process_alive(pid, starttime):
    info = _proc_info(pid)
    return info is not None and info[1] == starttime and info[0] != "Z"


def _fixture_daemons(roots):
    if isinstance(roots, (str, os.PathLike)):
        roots = [roots]
    roots = [Path(root).resolve() for root in roots]
    daemons = []
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            args = (entry / "cmdline").read_bytes().split(b"\0")
            env = (entry / "environ").read_bytes().split(b"\0")
        except (FileNotFoundError, PermissionError, ProcessLookupError):
            continue
        info = _proc_info(entry.name)
        if info is None or info[0] == "Z" or len(args) < 2 or Path(os.fsdecode(args[0])).name != "greenhouse" or args[1] != b"daemon":
            continue
        home = next((value.partition(b"=")[2] for value in env if value.startswith(b"GREENHOUSE_HOME=")), None)
        if home is None:
            continue
        try:
            fixture_home = Path(os.fsdecode(home)).resolve()
        except OSError:
            continue
        if any(fixture_home.is_relative_to(root) for root in roots):
            daemons.append((int(entry.name), os.fsdecode(home), info[1]))
    return daemons


def stop_expected_daemons(roots, timeout=5):
    if isinstance(roots, (str, os.PathLike)):
        roots = [roots]
    roots = [Path(root).resolve() for root in roots]
    found = _fixture_daemons(roots)
    for pid, _, starttime in found:
        if not _same_process_alive(pid, starttime):
            continue
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline and any(_same_process_alive(pid, starttime) for pid, _, starttime in found):
        time.sleep(0.05)


def sweep_daemons(roots, term_timeout=2, kill_timeout=2):
    if isinstance(roots, (str, os.PathLike)):
        roots = [roots]
    roots = [Path(root).resolve() for root in roots]
    seen = {}
    labels = ", ".join(str(root) for root in roots)

    for _ in range(3):
        found = _fixture_daemons(roots)
        if not found:
            break
        for pid, home, starttime in found:
            seen[(pid, starttime)] = home
            if not _same_process_alive(pid, starttime):
                continue
            print(f"daemon cleanup: terminating pid {pid} GREENHOUSE_HOME={home}", flush=True)
            try:
                os.kill(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass

        deadline = time.monotonic() + term_timeout
        while time.monotonic() < deadline and any(_same_process_alive(pid, starttime) for pid, _, starttime in found):
            time.sleep(0.05)

        for pid, home, starttime in found:
            if not _same_process_alive(pid, starttime):
                continue
            print(f"daemon cleanup: killing pid {pid} GREENHOUSE_HOME={home}", flush=True)
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

        deadline = time.monotonic() + kill_timeout
        while time.monotonic() < deadline and _fixture_daemons(roots):
            time.sleep(0.05)

    remaining = _fixture_daemons(roots)
    if remaining:
        details = ", ".join(f"pid {pid} GREENHOUSE_HOME={home}" for pid, home, _ in remaining)
        raise RuntimeError(f"fixture daemons survived cleanup under {labels}: {details}")
    if seen:
        details = ", ".join(f"pid {pid} GREENHOUSE_HOME={home}" for (pid, _), home in seen.items())
        raise RuntimeError(f"fixture daemons required cleanup under {labels}: {details}")
