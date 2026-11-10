class FloatConverter(BaseConverter):
    """A :class:`float` value. Available as ``float`` in rules.

    Values must have an integer and decimal part, ``4.`` and ``.4`` are not
    allowed. Scientific notation, ``inf``, and ``nan`` are not allowed.

    Values that are too large to fit in a float are not allowed; Python would
    convert them to ``inf``. Values that are too tiny are allowed; Python
    converts them to ``0.0``.

    Extraneous leading and trailing zeros are allowed. Many values, such as
    ``0.1``, are not exactly expressible as a float and are converted to the
    nearest expressible value. Therefore, there is not a 1-to-1 unique mapping
    between URLs and parsed values.

    :param map: The map this rule is bound to.
    :param min: The minimum value, inclusive.
    :param max: The maximum value, inclusive.
    :param signed: Allow negative values.

    .. versionchanged:: 3.2
        Non-ASCII digits are not allowed. Values that are too large and overflow
        to ``inf`` are not allowed. ``to_url`` performs validation and does not
        produce scientific notation.

    .. versionchanged:: 0.15
        The ``signed`` parameter was added.
    """

    weight = 50
    regex = r"[0-9]+\.[0-9]+"

    def __init__(
        self,
        map: Map,
        min: float | None = None,
        max: float | None = None,
        signed: bool = False,
    ) -> None:
        super().__init__(map)
        self.min = min
        self.max = max
        self.signed = signed

        if signed:
            self.regex = f"-?{self.regex}"

    def to_python(self, value: str) -> t.Any:
        value_num: float = float(value)
        # will convert too tiny to 0.0, too large to inf

        if math.isinf(value_num):
            raise ValidationError("Too large.")

        if (self.min is not None and value_num < self.min) or (
            self.max is not None and value_num > self.max
        ):
            raise ValidationError("Outside of allowed range.")

        return value_num

    def to_url(self, value: t.Any) -> str:
        value = float(value)

        if not math.isfinite(value):
            raise ValidationError("Infinity or NaN are not allowed.")

        if not self.signed and value < 0:
            raise ValidationError("Negative values are not allowed.")

        if (self.min is not None and value < self.min) or (
            self.max is not None and value > self.max
        ):
            raise ValidationError("Outside of allowed range.")

        # Use `str(value)` if it doesn't produce scientific notation.
        if "e" not in (value_str := str(value)):
            return value_str

        sig, _, exp_str = value_str.partition("e")
        left, _, right = sig.partition(".")
        exp = int(exp_str)

        # A big number. Expand trailing zeros in the integer part.
        if exp > 0:
            return f"{left}{right}{'0' * (exp - len(right))}.0"

        # A small number. Expand leading zeros in the fraction part.
        if left.startswith("-"):
            left = left[1:]
            start = "-0"
        else:
            start = "0"

        return f"{start}.{'0' * (-exp - len(left))}{left}{right}"

