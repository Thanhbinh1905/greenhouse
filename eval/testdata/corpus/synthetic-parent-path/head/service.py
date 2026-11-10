from pathlib import PurePosixPath
def permitted(name):
    return not name.startswith("/")
