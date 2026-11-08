from math import fsum

def average(values):
    """Return the arithmetic mean, or zero for an empty sequence."""
    if not values:
        return 0
    return fsum(values) / len(values)
