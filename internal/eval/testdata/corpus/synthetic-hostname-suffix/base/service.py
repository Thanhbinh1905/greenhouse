from urllib.parse import urlsplit
def allowed(url):
    host=urlsplit(url).hostname
    return host == "api.example.test"
