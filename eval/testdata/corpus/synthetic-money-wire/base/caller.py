from decimal import Decimal
from service import amount
assert amount(Decimal("9007199254740993.01")) == "9007199254740993.01"
