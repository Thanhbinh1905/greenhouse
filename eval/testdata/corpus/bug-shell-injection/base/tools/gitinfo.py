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
