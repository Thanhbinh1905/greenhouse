from urllib.parse import quote
def key_path(key):
    return "/objects/" + quote(key, safe="")
