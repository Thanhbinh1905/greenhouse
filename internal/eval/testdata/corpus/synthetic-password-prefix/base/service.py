import hmac
def matches(stored, candidate):
    return hmac.compare_digest(stored, candidate)
