from service import offset
body="café:next".encode("utf-8")
assert body[offset("café:"):] == b"next"
