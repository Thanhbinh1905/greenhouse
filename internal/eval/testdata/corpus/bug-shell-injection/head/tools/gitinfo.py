import subprocess


def current_branch(repo):
    """Return the checked-out branch name of repo."""
    out = subprocess.run(
        ["git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
    )
    return out.stdout.strip()


def branch_log(repo, branch):
    """Return the one-line log of a branch name sent by the web client."""
    out = subprocess.run(
        f"git -C {repo} log --oneline {branch}",
        shell=True,
        check=True,
        capture_output=True,
        text=True,
    )
    return out.stdout.splitlines()
