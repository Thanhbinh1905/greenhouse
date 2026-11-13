from ..sansio.response import Response as _SansIOResponse

class Response(_SansIOResponse):
    def __init__(
        self,
        response: str | bytes | cabc.Iterable[str] | cabc.Iterable[bytes] | None = None,
        status: int | str | HTTPStatus | None = None,
        headers: cabc.Mapping[str, str | cabc.Iterable[str]]
        | cabc.Iterable[tuple[str, str]]
        | None = None,
        mimetype: str | None = None,
        content_type: str | None = None,
        direct_passthrough: bool = False,
    ) -> None:
        super().__init__(
            status=status,
            headers=headers,
            mimetype=mimetype,
            content_type=content_type,
        )

        self.direct_passthrough = direct_passthrough
        """Use :attr:`response` directly as the WSGI iterable, rather than
        passing it through :meth:`iter_encoded`. Should be enabled when
        ``response`` is a binary file. It is enabled automatically when using
        :func:`.send_file`.
        """

        self._on_close: list[t.Callable[[], t.Any]] = []

        # Can't call set_data(response) for all types here, it may clear the
        # content-type header.
        if response is None:
            self.response = []
        elif isinstance(response, (str, bytes)):
            self.set_data(response)
        else:
            self.response = response

    def call_on_close(self, func: t.Callable[[], t.Any]) -> t.Callable[[], t.Any]:
        """Adds a function to the internal list of functions that should
        be called as part of closing down the response.  Since 0.7 this
        function also returns the function that was passed so that this
        can be used as a decorator.

        .. versionadded:: 0.6
        """
        self._on_close.append(func)
        return func

    def set_data(self, value: str | bytes) -> None:
        """Set the :attr:`response` iterable and :attr:`content_length`. A
        string is encoded as UTF-8. The value is wrapped as a single-item
        sequence.nse.

        .. versionadded:: 0.9
        """
        if isinstance(value, str):
            value = value.encode()

        self.response = [value]

        if self._auto_length:
            self.content_length = len(value)

    def _ensure_sequence(self, mutable: bool = False) -> None:
        """Use before operations that need :attr:`response` to be a
        fixed-length sequence, while obeying flags that disable this behavior.
        Use :meth:`make_sequence` to override those flags.

        This causes ``response`` to be iterated and encoded immediately, which
        will be a problem with large or infinite streamed data.

        :param mutable: Convert to a mutable sequence.
        :raise RuntimeError: If it is not already a sequence and
            :attr:direct_passthrough` is enabled or
            :attr:`implicit_sequence_conversion` is disabled.

        .. versionadded:: 0.6
        """
        if self.is_sequence:
            if mutable and not isinstance(self.response, cabc.MutableSequence):
                self.response = list(self.response)  # type: ignore[assignment]

            return

        if self.direct_passthrough:
            raise RuntimeError(
                "Attempted implicit sequence conversion but the"
                " response object is in direct passthrough mode."
            )

        if not self.implicit_sequence_conversion:
            raise RuntimeError(
                "The response object required the iterable to be a"
                " sequence, but implicit conversion was disabled."
                " Call 'make_sequence()' manually."
            )

        self.make_sequence()

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

    def close(self) -> None:
        """Close the wrapped response if possible.  You can also use the object
        in a with statement which will automatically close it.

        .. versionadded:: 0.9
           Can now be used in a with statement.
        """
        if hasattr(self.response, "close"):
            self.response.close()
        for func in self._on_close:
            func()

    def freeze(self) -> None:
        """Attempt to make the response reusable. Calls :meth:`make_sequence`
        and :meth:`add_etag`, and sets :attr:`content_length`.

        An arbitrary response cannot be guaranteed reusable or pickleable, even
        with this method.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Only reuse simple responses that
            do not depend on the current request, are fixed-length, and do not
            have callbacks.

        .. versionchanged:: 2.1
            Removed the ``no_etag`` parameter.

        .. versionchanged:: 2.0
            An ``ETag`` header is always added.

        .. versionchanged:: 0.6
            The ``Content-Length`` header is set.
        """
        import warnings

        warnings.warn(
            "The 'freeze' method is deprecated and will be removed in Werkzeug 4.0."
            " Only reuse simple responses that do not depend on the current request,"
            " are fixed-length, and do not have callbacks.",
            DeprecationWarning,
            stacklevel=2,
        )
        self.make_sequence()
        self.content_length = self.calculate_content_length()
        self.add_etag()

    def get_wsgi_headers(self, environ: wt.WSGIEnvironment) -> Headers:
        """This is automatically called right before the response is started
        and returns headers modified for the given environment.  It returns a
        copy of the headers from the response with some modifications applied
        if necessary.

        For example the location header (if present) is joined with the root
        URL of the environment.  Also the content length is automatically set
        to zero here for certain status codes.

        .. versionchanged:: 0.6
           Previously that function was called `fix_headers` and modified
           the response object in place.  Also since 0.6, IRIs in location
           and content-location headers are handled properly.

           Also starting with 0.6, Werkzeug will attempt to set the content
           length if it is able to figure it out on its own.  This is the
           case if all the strings in the response iterable are already
           encoded and the iterable is buffered.

        :param environ: the WSGI environment of the request.
        :return: returns a new :class:`~werkzeug.datastructures.Headers`
                 object.
        """
        headers = Headers(self.headers)
        location: str | None = None
        content_location: str | None = None
        content_length: str | None = None
        status = self.status_code

        # iterate over the headers to find all values in one go.  Because
        # get_wsgi_headers is used each response that gives us a tiny
        # speedup.
        for key, value in headers:
            ikey = key.lower()
            if ikey == "location":
                location = value
            elif ikey == "content-location":
                content_location = value
            elif ikey == "content-length":
                content_length = value

        if location is not None:
            uri_location = iri_to_uri(location)

            if location != uri_location:
                import warnings

                warnings.warn(
                    "The 'Location' header must be a URI, but it was an IRI. Set"
                    " 'response.location' to handle that. An invalid header will not be"
                    " automatically corrected in Werkzeug 4.0.",
                    DeprecationWarning,
                    stacklevel=2,
                )

            if self.autocorrect_location_header:
                import warnings

                from .request import Request

                warnings.warn(
                    "Setting 'Response.autocorrect_location_header' is deprecated"
                    " and will be removed in Werkzeug 4.0. Set 'response.location'"
                    " directly if you need a specific format.",
                    DeprecationWarning,
                    stacklevel=2,
                )
                # Make the location header an absolute URL.
                current_url = iri_to_uri(Request(environ).base_url)
                uri_location = urljoin(current_url, uri_location)

            headers["Location"] = uri_location

        # make sure the content location is a URL
        if content_location is not None:
            uri_content_location = iri_to_uri(content_location)

            if content_location != uri_content_location:
                import warnings

                warnings.warn(
                    "The 'Content-Location' header must be a URI, but it was an IRI."
                    " Set 'response.content_location' to handle that. An invalid header"
                    " will not be automatically corrected in Werkzeug 4.0.",
                    DeprecationWarning,
                    stacklevel=2,
                )

            headers["Content-Location"] = uri_content_location

        if 100 <= status < 200 or status == 204:
            # Per section 3.3.2 of RFC 7230, "a server MUST NOT send a
            # Content-Length header field in any response with a status
            # code of 1xx (Informational) or 204 (No Content)."
            headers.remove("Content-Length")
        elif status == 304:
            # remove representation metadata
            # https://httpwg.org/specs/rfc9110.html#status.304
            # https://httpwg.org/specs/rfc9110.html#representation.metadata
            for name in (
                "content-type",
                "content-encoding",
                "content-language",
                "content-length",
            ):
                headers.remove(name)
        elif self._auto_length and content_length is None and self.is_sequence:
            headers["Content-Length"] = str(self.calculate_content_length())

        return headers

    def get_app_iter(self, environ: wt.WSGIEnvironment) -> cabc.Iterable[bytes]:
        """Returns the application iterator for the given environ.  Depending
        on the request method and the current status code the return value
        might be an empty response rather than the one from the response.

        If the request method is `HEAD` or the status code is in a range
        where the HTTP specification requires an empty response, an empty
        iterable is returned.

        .. versionadded:: 0.6

        :param environ: the WSGI environment of the request.
        :return: a response iterable.
        """
        status = self.status_code
        if (
            environ["REQUEST_METHOD"] == "HEAD"
            or 100 <= status < 200
            or status in (204, 304)
        ):
            iterable: t.Iterable[bytes] = ()
        elif self.direct_passthrough:
            return self.response  # type: ignore[return-value]
        else:
            iterable = self.iter_encoded()
        return ClosingIterator(iterable, self.close)

    def get_wsgi_response(
        self, environ: wt.WSGIEnvironment
    ) -> tuple[cabc.Iterable[bytes], str, list[tuple[str, str]]]:
        """Returns the final WSGI response as tuple.  The first item in
        the tuple is the application iterator, the second the status and
        the third the list of headers.  The response returned is created
        specially for the given environment.  For example if the request
        method in the WSGI environment is ``'HEAD'`` the response will
        be empty and only the headers and status code will be present.

        .. versionadded:: 0.6

        :param environ: the WSGI environment of the request.
        :return: an ``(app_iter, status, headers)`` tuple.
        """
        headers = self.get_wsgi_headers(environ)
        app_iter = self.get_app_iter(environ)
        return app_iter, self.status, headers.to_wsgi_list()

    def __call__(
        self, environ: wt.WSGIEnvironment, start_response: wt.StartResponse
    ) -> cabc.Iterable[bytes]:
        """Process this response as WSGI application.

        :param environ: the WSGI environment.
        :param start_response: the response callable provided by the WSGI
                               server.
        :return: an application iterator
        """
        app_iter, status, headers = self.get_wsgi_response(environ)
        start_response(status, headers)
        return app_iter

    def apply_conditions(self, request: Request, *, exists: bool = True) -> None:
        """Check preconditions from the request: :attr:`~.Request.if_match`,
        :attr:`~.Request.if_unmodified_since`, :attr:`~.Request.if_none_match`,
        and :attr:`~.Request.if_modified_since`. The response's :attr:`etag` and
        :attr:`last_modified` must already be set.

        Depending on the request details and the precondition, a failure will
        either raise :exc:`.PreconditionFailed`, or :attr:`status_code` will be
        set to 304. The response will handle clearing the data and content
        length for 304 automatically.

        Call :meth:`apply_range` after this, it will exit early if a condition
        was not met.

        :param request: The request to get the range from.
        :param exists: Whether the resource currently exists. Used when
            ``If-Match`` or ``If-None-Match`` are ``*``.
        :raises .PreconditionFailed: If a precondition fails and results in a
            412 instead of a 304.

        .. versionadded:: 3.2
        """
        from ..exceptions import PreconditionFailed

        etag = self.etag
        last_modified = self.last_modified

        # https://httpwg.org/specs/rfc9110.html#precedence
        # first evaluate one of if-match or if-unmodified-since
        if request.if_match:
            if not (
                exists
                if request.if_match.star_tag
                else request.if_match.contains_strong(etag)
            ):
                raise PreconditionFailed()
        else:
            if (
                request.if_unmodified_since is not None
                and last_modified is not None
                and last_modified > request.if_unmodified_since
            ):
                raise PreconditionFailed()

        # then evaluate one of if-none-match or if-modified-since
        if request.if_none_match:
            if (
                exists
                if request.if_none_match.star_tag
                else request.if_none_match.contains_weak(etag)
            ):
                if request.method not in {"GET", "HEAD"}:
                    raise PreconditionFailed()

                self.status_code = 304
        else:
            if (
                request.method in {"GET", "HEAD"}
                and last_modified is not None
                and request.if_modified_since is not None
                and last_modified <= request.if_modified_since
            ):
                self.status_code = 304

class ResponseStream:
    """A file descriptor like object used by :meth:`Response.stream` to
    represent the body of the stream. It directly pushes into the
    response iterable of the response object.
    """

    mode = "wb+"

    def __init__(self, response: Response):
        self.response = response
        self.closed = False

    def write(self, value: bytes) -> int:
        if self.closed:
            raise ValueError("I/O operation on closed file")
        self.response._ensure_sequence(mutable=True)
        self.response.response.append(value)  # type: ignore[union-attr]
        self.response.content_length = None
        return len(value)

    def writelines(self, seq: cabc.Iterable[bytes]) -> None:
        for item in seq:
            self.write(item)

    def close(self) -> None:
        self.closed = True

    def flush(self) -> None:
        if self.closed:
            raise ValueError("I/O operation on closed file")

    def isatty(self) -> bool:
        if self.closed:
            raise ValueError("I/O operation on closed file")
        return False

    def tell(self) -> int:
        self.response._ensure_sequence()
        return sum(map(len, self.response.response))

    @property
    def encoding(self) -> str:
        return "utf-8"
