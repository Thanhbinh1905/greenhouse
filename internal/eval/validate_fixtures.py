"""Run stored fixture contracts without downloading packages or calling models."""

import ast
import __future__
import math
import os
from pathlib import Path
import subprocess
import sys
import types


CORPUS = Path(__file__).parent / "testdata" / "corpus"


def run_contracts():
    count = 0
    for case in sorted(CORPUS.glob("synthetic-*")):
        for tree, passes in (("base", True), ("head", False)):
            result = subprocess.run(
                [sys.executable, "-B", "caller.py"],
                cwd=case / tree,
                capture_output=True,
                text=True,
                env={**os.environ, "PYTHONDONTWRITEBYTECODE": "1"},
                timeout=10,
            )
            if (result.returncode == 0) != passes:
                raise AssertionError(f"{case.name}/{tree}: {result.stderr}")
            count += 1
    print(f"Python contracts: {count} base/head paths passed")


def load_function(path, name, namespace):
    tree = ast.parse(path.read_text())
    functions = [
        node for node in ast.walk(tree)
        if isinstance(node, ast.FunctionDef) and node.name == name
    ]
    if len(functions) != 1:
        raise AssertionError(f"{path}: expected one {name} declaration")
    module = ast.Module(body=functions, type_ignores=[])
    exec(compile(module, str(path), "exec", flags=__future__.annotations.compiler_flag), namespace)
    return namespace[name]


def public_float_contract():
    for tree in ("base", "head"):
        path = CORPUS / "public-reverse-negative-float" / tree / "src/werkzeug/routing/converters.py"
        to_url = load_function(path, "to_url", {"math": math})
        converter = types.SimpleNamespace(signed=True, min=None, max=None)
        result = to_url(converter, -1e-7)
        if (result == "-0.0000001") != (tree == "base"):
            raise AssertionError(f"{path}: unexpected conversion {result!r}")
    print("Public FloatConverter method: base passes, reversed head fails")


def public_doc_contract():
    for tree in ("base", "head"):
        path = CORPUS / "public-reverse-indent-unwind" / tree / "packages/zod/src/v4/core/doc.ts"
        script = """
const { Doc } = await import(process.argv[1]);
const doc = new Doc();
try { doc.indented(() => { throw new Error('unsupported child'); }); } catch {}
if ((doc.indent === 0) !== (process.argv[2] === 'base')) process.exit(1);
"""
        subprocess.run(
            ["node", "--experimental-strip-types", "--input-type=module", "-e", script, path.resolve().as_uri(), tree],
            check=True,
            capture_output=True,
            text=True,
            timeout=10,
        )
    print("Public Doc.indented method: base unwinds, reversed head retains indentation")


def judgment_modules():
    count = 0
    for case in sorted(CORPUS.glob("judgment-*")):
        for tree in ("base", "head"):
            subprocess.run(
                ["node", "--experimental-strip-types", str(case / tree / "policy.ts")],
                check=True,
                capture_output=True,
                text=True,
                timeout=10,
            )
            count += 1
    print(f"TypeScript judgment modules: {count} loaded")


if __name__ == "__main__":
    run_contracts()
    public_float_contract()
    public_doc_contract()
    judgment_modules()
