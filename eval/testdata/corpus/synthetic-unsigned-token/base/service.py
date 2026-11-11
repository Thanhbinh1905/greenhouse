def accept(header, verified):
    return header.get("alg") == "HS256" and verified
