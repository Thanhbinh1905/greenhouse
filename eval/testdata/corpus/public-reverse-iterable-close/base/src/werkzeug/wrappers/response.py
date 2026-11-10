class Response(_SansIOResponse):
    def make_sequence(self) -> None:
        """Convert the :attr:`response` iterator to a fixed-length sequence if
        it is not already.

        Sets :attr:`content_length` unless :attr:`automatically_set_content_length`
        is disabled.

        If the original ``response`` has a ``close`` method, it will be called
        after iterating.

        Some other methods automatically do this conversion, but will raise an
        error instead if :attr:`direct_passthrough` is enabled or
        :attr:`implicit_sequence_conversion` is disabled. Call this method to
        override those checks. This causes ``response`` to be iterated and
        encoded immediately, which will be a problem with large or infinite
        streamed data.

        .. versionchanged:: 3.2
            ``close`` is called immediately instead of with the response.

        .. versionadded:: 0.6
        """
        if not self.is_sequence:
            close: t.Callable[[], None] | None = getattr(self.response, "close", None)
            self.response = list(self.iter_encoded())

            if close is not None:
                close()

