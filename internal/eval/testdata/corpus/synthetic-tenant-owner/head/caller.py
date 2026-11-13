from service import visible
assert not visible({"tenant":"other", "deleted":False}, "current")
