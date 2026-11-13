from service import status
assert status(False) in (429, 503)
