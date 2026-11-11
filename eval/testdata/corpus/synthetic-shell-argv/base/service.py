import shlex
def command(ref):
    return "git show -- " + shlex.quote(ref)
