class MultiDict(TypeConversionDict[K, V]):
    def getlist(self, key: K, type: type[T] | None = None) -> list[V] | list[T]:
        """Return the list of items for a given key. If that key is not in the
        `MultiDict`, the return value will be an empty list.  Just like `get`,
        `getlist` accepts a `type` parameter.  All items will be converted
        with the callable defined there.

        :param key: The key to be looked up.
        :param type: A callable that is used to cast the value in the
                     :class:`MultiDict`.  If a :exc:`ValueError` is raised
                     by this callable the value will be removed from the list.
        :return: a :class:`list` of all the values for the key.
        """
        try:
            rv: list[V] = super().__getitem__(key)  # type: ignore[assignment]
        except KeyError:
            return []
        if type is None:
            return list(rv)
        result = []
        for item in rv:
            try:
                result.append(type(item))  # type: ignore[call-arg]
            except ValueError:
                pass
        return result

class _OrderedMultiDict(MultiDict[K, V]):
    def getlist(self, key: K, type: type[T] | None = None) -> list[V] | list[T]:
        rv: list[_omd_bucket[K, V]]

        try:
            rv = dict.__getitem__(self, key)  # type: ignore[index]
        except KeyError:
            return []
        if type is None:
            return [x.value for x in rv]
        result = []
        for item in rv:
            try:
                result.append(type(item.value))  # type: ignore[call-arg]
            except ValueError:
                pass
        return result

class CombinedMultiDict(ImmutableMultiDictMixin[K, V], MultiDict[K, V]):  # type: ignore[misc]
    """A read only :class:`MultiDict` that you can pass multiple :class:`MultiDict`
    instances as sequence and it will combine the return values of all wrapped
    dicts:

    >>> from werkzeug.datastructures import CombinedMultiDict, MultiDict
    >>> post = MultiDict([('foo', 'bar')])
    >>> get = MultiDict([('blub', 'blah')])
    >>> combined = CombinedMultiDict([get, post])
    >>> combined['foo']
    'bar'
    >>> combined['blub']
    'blah'

    This works for all read operations and will raise a `TypeError` for
    methods that usually change data which isn't possible.

    From Werkzeug 0.3 onwards, the `KeyError` raised by this class is also a
    subclass of the :exc:`~exceptions.BadRequest` HTTP exception and will
    render a page for a ``400 BAD REQUEST`` if caught in a catch-all for HTTP
    exceptions.
    """

    def __reduce_ex__(self, protocol: t.SupportsIndex) -> t.Any:
        return type(self), (self.dicts,)

    def __init__(self, dicts: cabc.Iterable[MultiDict[K, V]] | None = None) -> None:
        super().__init__()
        self.dicts: list[MultiDict[K, V]] = list(dicts or ())

    @classmethod
    def fromkeys(cls, keys: t.Any, value: t.Any = None) -> t.NoReturn:
        raise TypeError(f"cannot create {cls.__name__!r} instances by fromkeys")

    def __getitem__(self, key: K) -> V:
        for d in self.dicts:
            if key in d:
                return d[key]
        raise exceptions.BadRequestKeyError(key)

    @t.overload  # type: ignore[override]
    def get(self, key: K) -> V | None: ...
    @t.overload
    def get(self, key: K, default: V) -> V: ...
    @t.overload
    def get(self, key: K, default: T) -> V | T: ...
    @t.overload
    def get(self, key: str, type: type[T]) -> T | None: ...
    @t.overload
    def get(self, key: str, default: T, type: type[T]) -> T: ...
    def get(  # type: ignore[misc]
        self, key: K, default: V | T | None = None, type: type[T] | None = None
    ) -> V | T | None:
        for d in self.dicts:
            if key in d:
                if type is not None:
                    try:
                        return type(d[key])  # type: ignore[call-arg]
                    except ValueError:
                        continue
                return d[key]
        return default

    @t.overload
    def getlist(self, key: K) -> list[V]: ...
    @t.overload
    def getlist(self, key: K, type: type[T]) -> list[T]: ...
    def getlist(self, key: K, type: type[T] | None = None) -> list[V] | list[T]:
        rv = []
        for d in self.dicts:
            rv.extend(d.getlist(key, type))  # type: ignore[arg-type]
        return rv

    def _keys_impl(self) -> set[K]:
        """This function exists so __len__ can be implemented more efficiently,
        saving one list creation from an iterator.
        """
        return set(k for d in self.dicts for k in d)

    def keys(self) -> cabc.Iterable[K]:  # type: ignore[override]
        return self._keys_impl()

    def __iter__(self) -> cabc.Iterator[K]:
        return iter(self._keys_impl())

    @t.overload  # type: ignore[override]
    def items(self) -> cabc.Iterable[tuple[K, V]]: ...
    @t.overload
    def items(self, multi: t.Literal[True]) -> cabc.Iterable[tuple[K, list[V]]]: ...
    def items(
        self, multi: bool = False
    ) -> cabc.Iterable[tuple[K, V]] | cabc.Iterable[tuple[K, list[V]]]:
        found = set()
        for d in self.dicts:
            for key, value in d.items(multi):
                if multi:
                    yield key, value
                elif key not in found:
                    found.add(key)
                    yield key, value

    def values(self) -> cabc.Iterable[V]:  # type: ignore[override]
        for _, value in self.items():
            yield value

    def lists(self) -> cabc.Iterable[tuple[K, list[V]]]:
        rv: dict[K, list[V]] = {}
        for d in self.dicts:
            for key, values in d.lists():
                rv.setdefault(key, []).extend(values)
        return rv.items()

    def listvalues(self) -> cabc.Iterable[list[V]]:
        return (x[1] for x in self.lists())

    def copy(self) -> MultiDict[K, V]:  # type: ignore[override]
        """Return a shallow mutable copy of this object.

        This returns a :class:`MultiDict` representing the data at the
        time of copying. The copy will no longer reflect changes to the
        wrapped dicts.

        .. versionchanged:: 0.15
            Return a mutable :class:`MultiDict`.
        """
        return MultiDict(self)

    def __len__(self) -> int:
        return len(self._keys_impl())

    def __contains__(self, key: K) -> bool:  # type: ignore[override]
        for d in self.dicts:
            if key in d:
                return True
        return False

    def __repr__(self) -> str:
        return f"{type(self).__name__}({self.dicts!r})"
