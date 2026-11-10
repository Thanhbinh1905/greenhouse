from pathlib import Path
import re
import subprocess
import sys

# Usage: python3 tools/notices.py <greenhouse binary> [--check]
# Writes THIRD_PARTY_NOTICES.md from the modules linked into the binary, or
# with --check fails when the file does not match them.

binary = sys.argv[1]
modcache = Path(subprocess.check_output(["go", "env", "GOMODCACHE"], text=True).strip())
deps = [line.split()[1:3] for line in subprocess.check_output(["go", "version", "-m", binary], text=True).splitlines() if line.split()[:1] == ["dep"]]
out = ["# Third-party notices", "", "The `greenhouse` executable links these modules. Each license text follows its module.", ""]
for module, version in deps:
    escaped = re.sub(r"[A-Z]", lambda m: "!" + m.group(0).lower(), module)
    directory = modcache / f"{escaped}@{version}"
    license = next(p for p in sorted(directory.iterdir()) if re.match(r"(?i)(licen[cs]e|copying)", p.name))
    out += [f"## {module} {version}", "", "```text", license.read_text().strip(), "```", ""]
text = "\n".join(out)
target = Path(__file__).resolve().parent.parent / "THIRD_PARTY_NOTICES.md"
if sys.argv[2:3] == ["--check"]:
    if not target.exists() or target.read_text() != text:
        sys.exit("THIRD_PARTY_NOTICES.md does not match the linked modules; rerun tools/notices.py")
    print(f"PASS: notices cover {len(deps)} linked modules")
else:
    target.write_text(text)
