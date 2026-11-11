class Headers(cabc.MutableMapping[str, str]):
    def __or__(
        self, other: cabc.Mapping[str, t.Any | cabc.Collection[t.Any]]
    ) -> te.Self:
        if not isinstance(other, cabc.Mapping):
            return NotImplemented

        rv = self.copy()
        rv.update(other)
        return rv

    def __ior__(
        self,
        other: (
            cabc.Mapping[str, t.Any | cabc.Collection[t.Any]]
            | cabc.Iterable[tuple[str, t.Any]]
        ),
    ) -> te.Self:
        if not isinstance(other, (cabc.Mapping, cabc.Iterable)) or isinstance(
            other, str
        ):
            return NotImplemented

        self.update(other)
        return self

class EnvironHeaders(ImmutableHeadersMixin, Headers):  # type: ignore[misc]
    """Read only version of the headers from a WSGI environment.  This
    provides the same interface as `Headers` and is constructed from
    a WSGI environment.
    From Werkzeug 0.3 onwards, the `KeyError` raised by this class is also a
    subclass of the :exc:`~exceptions.BadRequest` HTTP exception and will
    render a page for a ``400 BAD REQUEST`` if caught in a catch-all for
    HTTP exceptions.
    """

    def __init__(self, environ: WSGIEnvironment) -> None:
        super().__init__()
        self.environ = environ

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, EnvironHeaders):
            return NotImplemented

        return self.environ is other.environ

    __hash__ = None  # type: ignore[assignment]

    def __getitem__(self, key: str) -> str:  # type: ignore[override]
        return self._get_key(key)

    def _get_key(self, key: str) -> str:
        if not isinstance(key, str):
            raise BadRequestKeyError(key)

        key = key.upper().replace("-", "_")

        if key in {"CONTENT_TYPE", "CONTENT_LENGTH"}:
            return self.environ[key]  # type: ignore[no-any-return]

        return self.environ[f"HTTP_{key}"]  # type: ignore[no-any-return]

    def __len__(self) -> int:
        return sum(1 for _ in self)

    def __iter__(self) -> cabc.Iterator[tuple[str, str]]:  # type: ignore[override]
        for key, value in self.environ.items():
            if key.startswith("HTTP_") and key not in {
                "HTTP_CONTENT_TYPE",
                "HTTP_CONTENT_LENGTH",
            }:
                yield key[5:].replace("_", "-").title(), value
            elif key in {"CONTENT_TYPE", "CONTENT_LENGTH"} and value:
                yield key.replace("_", "-").title(), value

    def copy(self) -> t.NoReturn:
        raise TypeError(f"cannot create {type(self).__name__!r} copies")

    def __or__(self, other: t.Any) -> t.NoReturn:
        raise TypeError(f"cannot create {type(self).__name__!r} copies")
