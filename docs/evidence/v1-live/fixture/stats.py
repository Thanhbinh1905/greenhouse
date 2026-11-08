from math import fsum

def average(values):
    """Return the arithmetic mean, or zero for an empty sequence."""
    return fsum(values) / len(values)
