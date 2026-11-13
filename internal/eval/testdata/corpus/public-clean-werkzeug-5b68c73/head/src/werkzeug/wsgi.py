class ClosingIterator:
    """Wrap an iterable that may or may not have a ``close`` method, adding that
    and additional functions to its own ``close`` method.

    Rather than using this directly, build a :class:`.Response` and use its
    :meth:`~.Response.call_on_close` method to add cleanup functions. It will
    handle creating the closing iterator.

    If a WSGI application returns an iterable with a ``close`` method, it
    will be called by the server at the end of the response. This class can be
    used to add additional cleanup when receiving an iterable from some other
    code.

    This does not handle the case where an exception interrupts the application
    before it returns the iterable. A higher level wrapper to handle safe
    execution and resource cleanup is needed.
    """

    def __init__(
        self,
        iterable: t.Iterable[bytes],
        callbacks: t.Callable[[], None]
        | t.Iterable[t.Callable[[], None]]
        | None = None,
    ) -> None:
        iterator = iter(iterable)
        self._next = t.cast(t.Callable[[], bytes], partial(next, iterator))
        if callbacks is None:
            callbacks = []
        elif callable(callbacks):
            callbacks = [callbacks]
        else:
            callbacks = list(callbacks)
        iterable_close = getattr(iterable, "close", None)
        if iterable_close:
            callbacks.insert(0, iterable_close)
        self._callbacks = callbacks

    def __iter__(self) -> ClosingIterator:
        return self

    def __next__(self) -> bytes:
        return self._next()

    def close(self) -> None:
        for callback in self._callbacks:
            callback()
