from service import inside
assert not inside("/srv/uploads", "/srv/uploads-private/token")
