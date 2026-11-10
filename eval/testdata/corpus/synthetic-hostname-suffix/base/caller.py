from service import allowed
assert not allowed("https://api.example.test.evil.test/private")
