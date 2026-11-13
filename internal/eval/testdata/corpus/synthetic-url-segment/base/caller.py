from service import key_path
assert key_path("tenant/file").split("/") == ["", "objects", "tenant%2Ffile"]
