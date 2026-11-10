from ..sansio.response import Response as SansIOResponse

class Response(SansIOResponse):
    def __init_subclass__(cls, **kwargs: t.Any) -> None:
        super().__init_subclass__(**kwargs)

        for name, alt in (
            ("get_wsgi_headers", "apply_status"),
            ("get_app_iter", "get_wsgi_response"),
        ):
            if getattr(cls, name) is not getattr(Response, name):
                import warnings

                warnings.warn(
                    f"The '{name}' method is deprecated and will be removed in"
                    f" Werkzeug 4.0. Override '{alt}' instead.",
                    DeprecationWarning,
                    stacklevel=2,
                )
                setattr(cls, f"_override_{name}", True)

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

        If this is set, the response's :meth:`close` method and
        :meth:`call_on_close` callbacks are not called at the end of the
        response.
        """

        self._on_close: list[t.Callable[[], t.Any]] = []

        if isinstance(response, (str, bytes)):
            self.set_data(response)
        elif response is not None:
            self.response = response

    def call_on_close(self, func: t.Callable[[], t.Any]) -> t.Callable[[], t.Any]:
        """Register additional functions to call when :meth:`close` is called.
        The WSGI server calls ``close`` when the response is done (or failed).

        If :attr:`direct_passthrough` is enabled, any registered functions will
        not be called.

        .. versionchanged:: 0.7
            Can be used as a decorator.

        .. versionadded:: 0.6
        """
        self._on_close.append(func)
        return func

    def _clear_body(self) -> None:
        """Clear :attr:`response`, calling its ``close`` method first.

        .. versionadded:: 3.2

        :meta private:
        """
        if (close := getattr(self.response, "close", None)) is not None:
            close()

        self.response = []

    def set_data(self, value: str | bytes) -> None:
        """Set the :attr:`response` iterable and :attr:`content_length`. A
        string is encoded as UTF-8. The value is wrapped as a single-item
        sequence.

        If the original ``response`` has a ``close`` method, it will be called
        before being replaced.

        .. versionchanged:: 3.2
            When replacing an existing iterable, calls its ``close`` method.

        .. versionadded:: 0.9
        """
        self._clear_body()

        if isinstance(value, str):
            value = value.encode()

        self.response = [value]

        if self._auto_length:
            self.content_length = len(value)

    def _ensure_sequence(self, mutable: None = None) -> None:
        """Use before operations that need :attr:`response` to be a
        fixed-length sequence, while obeying flags that disable this behavior.
        Use :meth:`make_sequence` to skip those checks.

        This causes ``response`` to be iterated and encoded immediately, which
        will be a problem with large or infinite streamed data.

        :raise RuntimeError: If it is not already a sequence and
            :attr:`direct_passthrough` is enabled or
            :attr:`implicit_sequence_conversion` is disabled.

        .. versionchanged:: 3.2
            The ``mutable`` parameter is deprecated and will be removed in
            Werkzeug 4.0. Build the sequence before constructing the response,
            rather than mutating it after.

        .. versionadded:: 0.6
        """
        if mutable is not None:
            import warnings

            warnings.warn(
                "The 'mutable' parameter is deprecated and will be removed in"
                " Werkzeug 4.0. Build the sequence before constructing the"
                " response, rather than mutating it after.",
                DeprecationWarning,
                stacklevel=2,
            )

        if self.is_sequence:
            if mutable and not isinstance(self.response, cabc.MutableSequence):
                self.response = list(self.response)

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
        """Convert the :attr:`response` iterator to a fixed-length sequence. If
        it is already a sequence, it is left as-is. Otherwise, all items are
        joined into a single bytes value and :attr:`content_length` is set.

        If the original ``response`` has a ``close`` method, it will be called
        after iterating.

        Some other methods automatically do this conversion, but will raise an
        error instead if :attr:`direct_passthrough` is enabled or
        :attr:`implicit_sequence_conversion` is disabled. Call this method to
        skip those checks. This causes ``response`` to be iterated and encoded
        immediately, which will be a problem with large or infinite streamed
        data.

        .. versionchanged:: 3.2
            ``close`` is called immediately instead of with the response.

            Joins all items into a single bytes value.

        .. versionadded:: 0.6
        """
        if not self.is_sequence:
            self.set_data(b"".join(self.iter_encoded()))

    def close(self) -> None:
        """Call the :attr:`response` iterable's ``close`` method if it has one.
        Then calls each function registered with :meth:`call_on_close`.

        This is called automatically by the WSGI server after the response is
        done. However, if :attr:`direct_passthrough` is enabled, this is not
        used.

        .. versionadded:: 0.9
           Can be used as a ``with`` context manager.
        """
        if (close := getattr(self.response, "close", None)) is not None:
            close()

        for func in self._on_close:
            func()

    def freeze(self) -> None:
        """Attempt to make the response reusable. An arbitrary response cannot
        be guaranteed reusable or pickleable, even with this method.

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
        self.add_etag()

    def get_wsgi_headers(self, environ: wt.WSGIEnvironment) -> Headers:
        """Modify and return the headers. Called by :meth:`get_wsgi_response`.

        -   ``Location`` and ``Content-Location`` are converted from IRI to URI.
        -   ``Location`` is made absolute if :attr:`autocorrect_location_header`
            is enabled.
        -   Calls :meth:`apply_status`.

        :param environ: The WSGI environment.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Use ``apply_status`` then access
            ``headers`` instead.

            Modifies headers and body in place based on status.

            Converting from IRI to URI is deprecated and will be removed in
            Werkzeug 4.0. Set the ``location`` and ``content_location``
            properties to set these values correctly.

            ``autocorrect_location_header`` header is deprecated and will be
            removed in Werkzeug 4.0. Set the ``location`` property if you need a
            specific format.

            ``automatically_set_content_length`` is deprecated and will be
            removed in Werkzeug 4.0. ``Content-Length`` will always be set if
            the response iterable is a sequence.

        .. versionchanged:: 0.6
            Renamed from ``fix_headers``. No longer modifies the response in
            place.

            ``Location`` and ``Content-Location`` are converted from IRI to URI.
            ``Content-Length`` is set if the response iterable is a sequence.
        """
        import warnings

        warnings.warn(
            "The 'get_wsgi_headers' method is deprecated and will be removed in"
            " Werkzeug 4.0. Use 'apply_status' then access 'headers' instead.",
            DeprecationWarning,
            stacklevel=2,
        )
        self._get_wsgi_headers(environ)
        self.apply_status()
        return self.headers

    def _get_wsgi_headers(self, environ: wt.WSGIEnvironment) -> Headers:
        if (location := self.headers.get("Location")) is not None:
            uri_location = iri_to_uri(location)

            if location != uri_location:
                import warnings

                warnings.warn(
                    "The 'Location' header must be a URI, but it was an IRI. Set"
                    " 'response.location' to handle that. An invalid header will not be"
                    " automatically corrected in Werkzeug 4.0.",
                    DeprecationWarning,
                    stacklevel=1,
                )
                self.headers["Location"] = uri_location

            if self.autocorrect_location_header:
                import warnings

                from .request import Request

                warnings.warn(
                    "Setting 'Response.autocorrect_location_header' is deprecated"
                    " and will be removed in Werkzeug 4.0. Set 'response.location'"
                    " directly if you need a specific format.",
                    DeprecationWarning,
                    stacklevel=1,
                )
                # Make the location header an absolute URL.
                current_url = iri_to_uri(Request(environ).base_url)
                self.headers["Location"] = urljoin(current_url, uri_location)

        # make sure the content location is a URL
        if (content_location := self.headers.get("Content-Location")) is not None:
            uri_content_location = iri_to_uri(content_location)

            if content_location != uri_content_location:
                import warnings

                warnings.warn(
                    "The 'Content-Location' header must be a URI, but it was an IRI."
                    " Set 'response.content_location' to handle that. An invalid header"
                    " will not be automatically corrected in Werkzeug 4.0.",
                    DeprecationWarning,
                    stacklevel=1,
                )
                self.headers["Content-Location"] = uri_content_location

        return self.headers

    def get_app_iter(self, environ: wt.WSGIEnvironment) -> cabc.Iterable[bytes]:
        """Return the body iterator for the response based on the request.

        -   Calls :meth:`apply_status`.
        -   A body is not sent for the ``HEAD`` request method.

        If :attr:`direct_passthrough` is enabled, :attr:`response` is returned
        directly, and will not call the response's :meth:`close` and
        :meth:`call_on_close` callbacks.

        :param environ: The WSGI environment.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Use ``get_wsgi_response`` instead.

        .. versionadded:: 0.6
        """
        import warnings

        warnings.warn(
            "The 'get_app_iter' method is deprecated and will be removed in"
            " Werkzeug 4.0. Use 'get_wsgi_response' instead.",
            DeprecationWarning,
            stacklevel=2,
        )
        self.apply_status()
        return self._get_app_iter(environ)

    def _get_app_iter(self, environ: wt.WSGIEnvironment) -> t.Iterable[bytes]:
        if environ.get("REQUEST_METHOD") == "HEAD":
            iterable: t.Iterable[bytes] = ()
        elif self.direct_passthrough:
            return self.response  # type: ignore[return-value]
        else:
            iterable = self.iter_encoded()

        return ClosingIterator(iterable, self.close)

    def apply_status(self) -> None:
        """Adjust the response headers and body based on its status code.

        -   Remove body and ``Content-Length`` for 1XX, 204, 205, and 304.
        -   Remove additional representation metadata (``Content-Type``,
            ``Content-Encoding``, ``Content-Language``) for 304.
        -   Set ``Content-Length`` if the body is a sequence for other statuses.

        .. versionadded:: 3.2
        """
        code = self.status_code

        if 100 <= code < 200 or code in {204, 205, 304}:
            self.headers.remove("Content-Length")
            self._clear_body()
        elif self._auto_length and self.content_length is None and self.is_sequence:
            self.content_length = self.calculate_content_length()

        if code == 304:
            # remove representation metadata
            # https://httpwg.org/specs/rfc9110.html#status.304
            # https://httpwg.org/specs/rfc9110.html#representation.metadata
            for name in ("content-type", "content-encoding", "content-language"):
                self.headers.remove(name)

    def get_wsgi_response(
        self, environ: wt.WSGIEnvironment
    ) -> tuple[cabc.Iterable[bytes], str, list[tuple[str, str]]]:
        """Produce the data needed to send a WSGI response.

        Calls :meth:`apply_status`, which may modify the headers and body
        in place.

        Modifies the returned data based on the WSGI environment. For example,
        the returned iterable is empty for a ``HEAD`` request.

        The :attr:`response` iterable is wrapped to call :meth:`close` at the
        end of the response. However, if :attr:`direct_passthrough` is enabled,
        it is not wrapped and ``close`` will not be called.

        :param environ: The WSGI environment.
        :return: ``(app_iter, status, headers)``

        .. versionchanged:: 3.2
            Modifies the headers and body in place based on the status.

        .. versionadded:: 0.6
        """
        self.apply_status()

        if self._override_get_wsgi_headers:
            headers = self.get_wsgi_headers(environ)
        else:
            headers = self._get_wsgi_headers(environ)

        if self._override_get_app_iter:
            app_iter = self.get_app_iter(environ)
        else:
            app_iter = self._get_app_iter(environ)

        return app_iter, self.status, headers.to_wsgi_list()

    def __call__(
        self, environ: wt.WSGIEnvironment, start_response: wt.StartResponse
    ) -> cabc.Iterable[bytes]:
        """Process this response as WSGI application.

        :param environ: The WSGI environment for the request.
        :param start_response: The ``start_response`` callable provided by the
            WSGI server.
        :return: An iterable with the response body.
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
                self.apply_status()
        else:
            if (
                request.method in {"GET", "HEAD"}
                and last_modified is not None
                and request.if_modified_since is not None
                and last_modified <= request.if_modified_since
            ):
                self.status_code = 304
                self.apply_status()

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
        self.response._ensure_sequence(mutable=True)  # type: ignore[arg-type]
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

