#!/usr/bin/env python3
"""Run the fixed posse PR review set through Greenhouse AXI and Pi."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time


CASES = Path(__file__).with_name("posse_review_regression.json")


def matches(expected, result):
    findings = result.get("findings") or []
    if expected["kind"] == "clean":
        return result.get("verdict") == "clean"
    if expected["kind"] == "no-p1":
        if result.get("verdict") not in {"clean", "findings"}:
            return False
        return not any(
            item.get("priority") == "P1" and item.get("file") == expected["path"]
            and item.get("start", 0) <= expected["end"] and item.get("end", 0) >= expected["start"]
            for item in findings
        )
    return any(
        item.get("priority") == expected["priority"] and item.get("file") == expected["path"]
        and item.get("start", 0) <= expected["end"] and item.get("end", 0) >= expected["start"]
        for item in findings
    )


def invoke(binary, worktree, home, request):
    env = os.environ.copy()
    env["GREENHOUSE_HOME"] = str(home)
    result = subprocess.run(
        [str(binary), "axi"], cwd=worktree, env=env,
        input=json.dumps(request), text=True, capture_output=True, timeout=500,
    )
    if result.returncode:
        raise RuntimeError(f"greenhouse axi failed: {result.stderr.strip()}")
    response = json.loads(result.stdout)
    if response.get("outcome") != "ok":
        raise RuntimeError(f"greenhouse axi {request['op']} {response.get('outcome')}: {response.get('error')}")
    return response


def run(binary, source, output, case, repetition, model):
    name = f"{case['pr']}-{repetition}"
    worktree = output / "worktrees" / name
    home = output / "homes" / name
    result_path = output / "results" / f"{name}.json"
    if result_path.exists():
        raise RuntimeError(f"result already exists: {result_path}")
    worktree.parent.mkdir(parents=True, exist_ok=True)
    home.mkdir(parents=True, exist_ok=False, mode=0o700)
    subprocess.run(
        ["git", "-C", str(source), "worktree", "add", "--detach", str(worktree), case["head"]],
        check=True, capture_output=True, text=True,
    )
    host = {"name": "greenhouse-regression", "id": name}
    try:
        envelope = invoke(binary, worktree, home, {"version": 2, "op": "attach", "host": host})

        def mutate(op, suffix, **fields):
            nonlocal envelope
            request = {
                "version": 2, "op": op, "host": host,
                "run_id": envelope["run_id"], "input_id": envelope["input_id"],
                "gate_id": envelope["gate_id"], "expected": envelope["token"],
                "action_id": f"{name}-{suffix}", **fields,
            }
            envelope = invoke(binary, worktree, home, request)
            return envelope

        mutate("apply", "apply", reason="Fixed review regression baseline", settings={
            "mode": "ask", "preset": "one-pass", "limit": 3,
            "rules": ["unslop", "no-comments"], "reviewer_backend": "pi",
            "reviewer_model": model, "fixer_backend": "pi", "fixer_model": "",
        })
        if not mutate("resolve-rules", "rules").get("rules_ready"):
            raise RuntimeError(f"review criteria did not resolve for {name}")
        mutate("prepare", "prepare", intent=case["intent"], target=case["target"])
        mutate("review", "review", grant="round-1", reason="Fixed review regression baseline")
        deadline = time.monotonic() + 280
        while True:
            envelope = invoke(binary, worktree, home, {
                "version": 2, "op": "inspect", "run_id": envelope["run_id"], "host": host,
            })
            result = envelope.get("host_result") or {}
            if result.get("status") and result["status"] != "running":
                break
            if time.monotonic() >= deadline:
                raise TimeoutError(f"review did not settle for {name}")
            time.sleep(2)
        if result.get("head") != case["head"] or result.get("target") != case["target"] or not result.get("current"):
            raise RuntimeError(f"review evidence is not bound to the requested commits for {name}")
        result.update({"pr": case["pr"], "repetition": repetition, "expected": case["expected"],
                       "matched": matches(case["expected"], result)})
        result_path.parent.mkdir(parents=True, exist_ok=True)
        result_path.write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"pr": case["pr"], "run": repetition, "matched": result["matched"],
                          "verdict": result.get("verdict"),
                          "status": result.get("status"), "findings": result.get("findings", []),
                          "reason": result.get("reason")}), flush=True)
    finally:
        try:
            invoke(binary, worktree, home, {"version": 2, "op": "stop"})
        except Exception as error:
            print(f"could not stop Greenhouse home {home}: {error}", file=sys.stderr)
        if worktree.exists():
            subprocess.run(["git", "-C", str(source), "worktree", "remove", "--force", str(worktree)],
                           check=True, capture_output=True, text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path, help="built Greenhouse executable")
    parser.add_argument("--source", required=True, type=Path, help="local posse clone with the recorded commits")
    parser.add_argument("--out", required=True, type=Path, help="private task scratch directory for homes, worktrees and results")
    parser.add_argument("--runs", type=int, default=2)
    parser.add_argument("--model", default="openai/gpt-5.6-luna")
    parser.add_argument("--pr", action="append", help="run only this PR number; repeatable")
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    binary, source, output = (p.resolve() for p in (args.binary, args.source, args.out))
    cases = json.loads(CASES.read_text())
    chosen = [case for case in cases if not args.pr or case["pr"] in args.pr]
    if args.pr and {case["pr"] for case in chosen} != set(args.pr):
        parser.error("unknown PR number")
    for case in chosen:
        for repetition in range(1, args.runs + 1):
            run(binary, source, output, case, repetition, args.model)


if __name__ == "__main__":
    main()
