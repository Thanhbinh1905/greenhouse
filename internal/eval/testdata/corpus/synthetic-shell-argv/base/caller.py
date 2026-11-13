from service import command
import shlex
assert shlex.split(command("a; echo injected")) == ["git", "show", "--", "a; echo injected"]
