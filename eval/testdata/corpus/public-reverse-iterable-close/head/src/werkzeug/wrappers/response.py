class Response(_SansIOResponse):
    def make_sequence(self) -> None:
        """Converts the response iterator in a list.  By default this happens
        automatically if required.  If `implicit_sequence_conversion` is
        disabled, this method is not automatically called and some properties
        might raise exceptions.  This also encodes all the items.

        .. versionadded:: 0.6
        """
        if not self.is_sequence:
            # if we consume an iterable we have to ensure that the close
            # method of the iterable is called if available when we tear
            # down the response
            close = getattr(self.response, "close", None)
            self.response = list(self.iter_encoded())
            if close is not None:
                self.call_on_close(close)

