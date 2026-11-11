from service import render
assert "<script>" not in render("<script>alert(1)</script>")
