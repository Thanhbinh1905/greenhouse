from ..http import is_byte_range_valid

class Range:
    def range_for_length(self, length: int | None) -> tuple[int, int] | None:
        """Return the ``(start, stop)`` values to use for a
        :class:`.ContentRange` header with the given complete length. Or ``None``
        if a valid range cannot be constructed.

        This will generate a range regardless of the value of :attr:`units`.
        However, :meth:`.Response.make_conditional` can only handle ``bytes``.

        If there are multiple ranges in the header, this will only return the
        first range.

        :param length: The complete length of the content. ``None`` means the
            value is unknown, and will return ``None``.
        :return: ``(start, stop)`` if the range and length are valid, ``None``
            otherwise.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Use ``make_content_range`` instead.

        .. versionchanged:: 3.2
            Allows units other than ``bytes``. Will return the first range if
            there are multiple.
        """
        import warnings

        warnings.warn(
            "'range_for_length' is deprecated and will be removed in Werkzeug 4.0."
            " Use 'make_content_range` instead.",
            DeprecationWarning,
            stacklevel=2,
        )
        return self._private_range_for_length(length)

    def _private_range_for_length(self, length: int | None) -> tuple[int, int] | None:
        if length is None or not self.ranges:
            return None

        start, stop = self.ranges[0]

        if stop is None:
            stop = length

            if start < 0:
                start += length

        if not is_byte_range_valid(start, stop, length):
            return None

        return start, min(stop, length)

    def make_content_range(self, length: int | None) -> ContentRange | None:
        """Create a :class:`.ContentRange` with the given complete length. Or
        ``None`` if a valid range cannot be constructed.

        This will generate a range regardless of the value of :attr:`units`.
        However, :meth:`.Response.make_conditional` can only handle ``bytes``.

        If there are multiple ranges in the header, this will only return the
        first range.

        .. versionchanged:: 3.2
            Allows units other than ``bytes``. Will return the first range if
            there are multiple.
        """
        # TODO inline after deprecation
        if (bounds := self._private_range_for_length(length)) is None:
            return None

        return ContentRange(self.units, bounds[0], bounds[1], length)

    def to_content_range_header(self, length: int | None) -> str | None:
        """Converts the object into `Content-Range` HTTP header,
        based on given length

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Use ``make_content_range`` then
            call its ``to_header`` method instead.
        """
        import warnings

        warnings.warn(
            "'to_content_range_header' is deprecated and will be removed in"
            " Werkzeug 4.0. Use 'make_content_range` then call its 'to_header'"
            " method instead.",
            DeprecationWarning,
            stacklevel=2,
        )

        if (cr := self.make_content_range(length)) is None:
            return None

        return cr.to_header()
