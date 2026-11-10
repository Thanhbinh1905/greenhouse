class IfRange:
    """A parsed ``If-Range`` header. Either a strong ETag or a datetime, but not
    both. Weak ETag values must not be used.

    :attr:`.Request.if_range` returns an instance.

    :param etag: An unquoted strong ETag value.
    :param date: A timezone-aware datetime.

    .. versionadded:: 0.7
    """

    def __init__(self, etag: str | None = None, date: datetime | None = None):
        self.etag = etag
        """An unquoted strong ETag value."""

        self.date = date
        """A timezone-aware datetime."""

    @classmethod
    def from_header(cls, value: str | None) -> te.Self:
        """Parse an ``If-Range`` header value and create an instance of this
        class. A weak ETag value is discarded.

        .. versionadded:: 3.2
        """
        if not value:
            return cls()

        if (date := parse_date(value)) is not None:
            return cls(date=date)

        if (etag := ETag.from_header(value)) is None or etag.weak:
            return cls()

        return cls(etag=etag.value)

    def to_header(self) -> str:
        """Convert to an ``If-Range`` header value."""
        if self.date is not None:
            return http_date(self.date)

        if self.etag is not None:
            return ETag(self.etag).to_header()

        return ""

    def __bool__(self) -> bool:
        return self.etag is not None or self.date is not None

    def __str__(self) -> str:
        return self.to_header()

    def __repr__(self) -> str:
        return f"<{type(self).__name__} {str(self)!r}>"

