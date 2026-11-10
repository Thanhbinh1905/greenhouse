class ImmutableDictMixin(t.Generic[K, V]):
    """Makes a :class:`dict` immutable.

    .. versionchanged:: 3.1
        Disallow ``|=`` operator.

    .. versionadded:: 0.5

    :private:
    """

    _hash_cache: int | None = None

    @classmethod
    @t.overload
    def fromkeys(
        cls, keys: cabc.Iterable[K], value: None
    ) -> ImmutableDictMixin[K, t.Any | None]: ...
    @classmethod
    @t.overload
    def fromkeys(cls, keys: cabc.Iterable[K], value: V) -> ImmutableDictMixin[K, V]: ...
    @classmethod
    def fromkeys(
        cls, keys: cabc.Iterable[K], value: V | None = None
    ) -> ImmutableDictMixin[K, t.Any | None] | ImmutableDictMixin[K, V]:
        instance = super().__new__(cls)
        instance.__init__(zip(keys, repeat(value)))  # type: ignore[misc]
        return instance

    def __reduce_ex__(self, protocol: t.SupportsIndex) -> t.Any:
        return type(self), (dict(self),)  # type: ignore[call-overload]

    def _iter_hashitems(self) -> t.Iterable[t.Any]:
        return self.items()  # type: ignore[attr-defined,no-any-return]

    def __hash__(self) -> int:
        if self._hash_cache is not None:
            return self._hash_cache
        rv = self._hash_cache = hash(frozenset(self._iter_hashitems()))
        return rv

    def setdefault(self, key: t.Any, default: t.Any = None) -> t.NoReturn:
        _immutable_error(self)

    def update(self, arg: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def __ior__(self, other: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def pop(self, key: t.Any, default: t.Any = None) -> t.NoReturn:
        _immutable_error(self)

    def popitem(self) -> t.NoReturn:
        _immutable_error(self)

    def __setitem__(self, key: t.Any, value: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def __delitem__(self, key: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def clear(self) -> t.NoReturn:
        _immutable_error(self)

class ImmutableHeadersMixin:
    """Makes a :class:`Headers` immutable.  We do not mark them as
    hashable though since the only usecase for this datastructure
    in Werkzeug is a view on a mutable structure.

    .. versionchanged:: 3.1
        Disallow ``|=`` operator.

    .. versionadded:: 0.5

    :private:
    """

    def __delitem__(self, key: t.Any, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def __setitem__(self, key: t.Any, value: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def set(self, key: t.Any, value: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def setlist(self, key: t.Any, values: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def add(self, key: t.Any, value: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def add_header(self, key: t.Any, value: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def remove(self, key: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def extend(self, arg: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def update(self, arg: t.Any, /, **kwargs: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def __ior__(self, other: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def insert(self, pos: t.Any, value: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def pop(self, key: t.Any = None, default: t.Any = _missing) -> t.NoReturn:
        _immutable_error(self)

    def popitem(self) -> t.NoReturn:
        _immutable_error(self)

    def setdefault(self, key: t.Any, default: t.Any) -> t.NoReturn:
        _immutable_error(self)

    def setlistdefault(self, key: t.Any, default: t.Any) -> t.NoReturn:
        _immutable_error(self)

class UpdateDictMixin(dict[K, V]):
    """Makes dicts call `self.on_update` on modifications.

    .. versionchanged:: 3.1
        Implement ``|=`` operator.

    .. versionadded:: 0.5

    :private:
    """

    on_update: cabc.Callable[[te.Self], None] | None = None

    def setdefault(self: te.Self, key: K, default: V | None = None) -> V:
        modified = key not in self
        rv = super().setdefault(key, default)  # type: ignore[arg-type]
        if modified and self.on_update is not None:
            self.on_update(self)
        return rv

    @t.overload
    def pop(self: te.Self, key: K) -> V: ...
    @t.overload
    def pop(self: te.Self, key: K, default: V) -> V: ...
    @t.overload
    def pop(self: te.Self, key: K, default: T) -> T: ...
    def pop(
        self: te.Self,
        key: K,
        default: V | T = _missing,  # type: ignore[assignment]
    ) -> V | T:
        modified = key in self
        if default is _missing:
            rv = super().pop(key)
        else:
            rv = super().pop(key, default)  # type: ignore[arg-type]
        if modified and self.on_update is not None:
            self.on_update(self)
        return rv

    @_always_update
    def __setitem__(self, key: K, value: V) -> None:
        super().__setitem__(key, value)

    @_always_update
    def __delitem__(self, key: K) -> None:
        super().__delitem__(key)

    @_always_update
    def clear(self) -> None:
        super().clear()

    @_always_update
    def popitem(self) -> tuple[K, V]:
        return super().popitem()

    @_always_update
    def update(  # type: ignore[override]
        self,
        arg: cabc.Mapping[K, V] | cabc.Iterable[tuple[K, V]] | None = None,
        /,
        **kwargs: V,
    ) -> None:
        if arg is None:
            super().update(**kwargs)
        else:
            super().update(arg, **kwargs)

    @_always_update
    def __ior__(  # type: ignore[override]
        self, other: cabc.Mapping[K, V] | cabc.Iterable[tuple[K, V]]
    ) -> te.Self:
        return super().__ior__(other)

