from service import private_mode
assert private_mode() & 0o077 == 0
