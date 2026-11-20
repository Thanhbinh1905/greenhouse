import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

from daemon_cleanup import sweep_daemons


class SweepDaemonsTest(unittest.TestCase):
    def test_forced_failure_sweeps_daemon_in_finally(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home = root / "state"
            home.mkdir(mode=0o700)
            tools = Path(__file__).resolve().parent
            code = '''
import os, subprocess, sys, time
from pathlib import Path
sys.path.insert(0, sys.argv[1])
from daemon_cleanup import sweep_daemons
root = Path(sys.argv[2])
home = root / "state"
process = subprocess.Popen([os.environ["GREENHOUSE_TEST_BINARY"], "daemon"], cwd="/",
    env={**os.environ, "GREENHOUSE_HOME": str(home)}, stdout=subprocess.DEVNULL,
    stderr=subprocess.DEVNULL, start_new_session=True)
deadline = time.monotonic() + 10
while not (home / "pid").exists() and time.monotonic() < deadline:
    time.sleep(0.02)
assert (home / "pid").exists(), "fixture daemon did not start"
try:
    raise AssertionError("forced failure")
finally:
    sweep_daemons(root)
'''
            result = subprocess.run(
                [os.environ.get("PYTHON", "python3"), "-c", code, str(tools), str(root)],
                env={**os.environ, "GREENHOUSE_TEST_BINARY": os.environ["GREENHOUSE_TEST_BINARY"]},
                capture_output=True,
                text=True,
                timeout=15,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("forced failure", result.stderr)
            self.assertIn("fixture daemons required cleanup", result.stderr)
            self.assertIn("daemon cleanup: terminating pid", result.stdout)
            sweep_daemons(root)

    def test_stops_and_reports_fixture_daemon(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home = root / "state"
            home.mkdir(mode=0o700)
            process = subprocess.Popen(
                [os.environ["GREENHOUSE_TEST_BINARY"], "daemon"],
                cwd="/",
                env={**os.environ, "GREENHOUSE_HOME": str(home)},
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                start_new_session=True,
            )
            try:
                deadline = time.monotonic() + 10
                while not (home / "pid").exists() and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertTrue((home / "pid").exists(), "fixture daemon did not start")
                with self.assertRaisesRegex(RuntimeError, str(process.pid)):
                    sweep_daemons(root)
                self.assertIsNotNone(process.poll())
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=5)
                else:
                    process.wait()

    def test_clean_fixture_is_not_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            sweep_daemons(Path(directory))


if __name__ == "__main__":
    unittest.main()
