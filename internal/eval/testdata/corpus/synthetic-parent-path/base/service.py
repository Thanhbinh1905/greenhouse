from pathlib import PurePosixPath
def permitted(name):
    p=PurePosixPath(name)
    return not p.is_absolute() and ".." not in p.parts
