import posixpath
def inside(root, target):
    return posixpath.commonpath([root, target]) == root
