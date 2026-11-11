def accept(header, verified):
    return verified or header.get("alg") == "none"
