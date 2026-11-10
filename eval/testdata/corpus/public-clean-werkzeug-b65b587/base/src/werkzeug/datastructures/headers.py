class Headers(cabc.MutableMapping[str, str]):
    def to_wsgi_list(self) -> list[tuple[str, str]]:
        """Convert the headers into a list suitable for WSGI.

        :return: list
        """
        return list(self)  # type: ignore[arg-type]

