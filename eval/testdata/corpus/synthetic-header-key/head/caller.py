from service import headers
assert headers({"Content-Type": "text/plain"})["content-type"] == "text/plain"
