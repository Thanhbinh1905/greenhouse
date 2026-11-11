class MultiDict(TypeConversionDict[K, V]):
    def __or__(  # type: ignore[override]
        self, other: cabc.Mapping[K, V | cabc.Collection[V]]
    ) -> MultiDict[K, V]:
        if not isinstance(other, cabc.Mapping):
            return NotImplemented

        rv = self.copy()
        rv.update(other)
        return rv

    def __ior__(  # type: ignore[override]
        self,
        other: cabc.Mapping[K, V | cabc.Collection[V]] | cabc.Iterable[tuple[K, V]],
    ) -> te.Self:
        if not isinstance(other, (cabc.Mapping, cabc.Iterable)) or isinstance(
            other, str
        ):
            return NotImplemented

        self.update(other)
        return self
