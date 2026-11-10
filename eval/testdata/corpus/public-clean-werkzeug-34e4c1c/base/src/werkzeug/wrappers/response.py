from .._internal import _get_environ

from ..datastructures.etag import ETagSet

from ..datastructures.range import Range

from ..http import http_date

from ..http import is_resource_modified

class Response(_SansIOResponse):
    def _wrap_range_response(self, start: int, length: int) -> None:
        """Wrap existing Response in case of Range Request context."""
        if self.status_code == 206:
            self.response = _RangeWrapper(self.response, start, length)  # type: ignore[arg-type]

    def _is_range_request_processable(self, environ: WSGIEnvironment) -> bool:
        """Return ``True`` if `Range` header is present and if underlying
        resource is considered unchanged when compared with `If-Range` header.
        """
        return "HTTP_RANGE" in environ and (
            "HTTP_IF_RANGE" not in environ
            or not is_resource_modified(
                environ,
                self.headers.get("ETag"),
                last_modified=self.headers.get("Last-Modified"),
                ignore_if_range=False,
            )
        )

    def _process_range_request(
        self,
        environ: WSGIEnvironment,
        complete_length: int | None,
        accept_ranges: bool | str,
    ) -> bool:
        """Handle Range Request related headers (RFC7233).  If `Accept-Ranges`
        header is valid, and Range Request is processable, we set the headers
        as described by the RFC, and wrap the underlying response in a
        RangeWrapper.

        Returns ``True`` if Range Request can be fulfilled, ``False`` otherwise.

        :raises: :class:`~werkzeug.exceptions.RequestedRangeNotSatisfiable`
                 if `Range` header could not be parsed or satisfied.

        .. versionchanged:: 3.2
            Adds the ``Accept-Ranges`` header if ``accept_ranges`` is passed,
            even if this is not a satisfiable range request.

        .. versionchanged:: 2.0
            Returns ``False`` if the length is 0.
        """
        from ..exceptions import RequestedRangeNotSatisfiable

        if not accept_ranges:
            return False

        if accept_ranges is True:
            accept_ranges = "bytes"

        self.accept_ranges = accept_ranges

        if not (complete_length and self._is_range_request_processable(environ)):
            return False

        parsed_range = Range.from_header(environ.get("HTTP_RANGE"))

        if parsed_range is None:
            raise RequestedRangeNotSatisfiable(complete_length)

        content_range = parsed_range.make_content_range(complete_length)

        if content_range is None:
            raise RequestedRangeNotSatisfiable(complete_length)

        content_length = content_range.stop - content_range.start  # type: ignore[operator]
        self.content_length = content_length
        self.content_range = content_range
        self.status_code = 206
        self._wrap_range_response(content_range.start, content_length)  # type: ignore[arg-type]
        return True

    def make_conditional(
        self,
        request_or_environ: WSGIEnvironment | Request,
        accept_ranges: bool | str = False,
        complete_length: int | None = None,
    ) -> Response:
        """Make the response conditional to the request.  This method works
        best if an etag was defined for the response already.  The `add_etag`
        method can be used to do that.  If called without etag just the date
        header is set.

        This does nothing if the request method in the request or environ is
        anything but GET or HEAD.

        For optimal performance when handling range requests, it's recommended
        that your response data object implements `seekable`, `seek` and `tell`
        methods as described by :py:class:`io.IOBase`.  Objects returned by
        :meth:`~werkzeug.wsgi.wrap_file` automatically implement those methods.

        It does not remove the body of the response because that's something
        the :meth:`__call__` function does for us automatically.

        Returns self so that you can do ``return resp.make_conditional(req)``
        but modifies the object in-place.

        :param request_or_environ: a request object or WSGI environment to be
                                   used to make the response conditional
                                   against.
        :param accept_ranges: This parameter dictates the value of
                              `Accept-Ranges` header. If ``False`` (default),
                              the header is not set. If ``True``, it will be set
                              to ``"bytes"``. If it's a string, it will use this
                              value.
        :param complete_length: Will be used only in valid Range Requests.
                                It will set `Content-Range` complete length
                                value and compute `Content-Length` real value.
                                This parameter is mandatory for successful
                                Range Requests completion.
        :raises: :class:`~werkzeug.exceptions.RequestedRangeNotSatisfiable`
                 if `Range` header could not be parsed or satisfied.

        .. versionchangedd: 3.2
            Adds the ``Accept-Ranges`` header if ``accept_ranges`` is passed,
            even if this is not a satisfiable range request.

        .. versionchanged:: 2.0
            Range processing is skipped if length is 0 instead of
            raising a 416 Range Not Satisfiable error.
        """
        environ = _get_environ(request_or_environ)
        if environ["REQUEST_METHOD"] in ("GET", "HEAD"):
            # if the date is not in the headers, add it now.  We however
            # will not override an already existing header.  Unfortunately
            # this header will be overridden by many WSGI servers including
            # wsgiref.
            if "Date" not in self.headers:
                self.headers["Date"] = http_date()
            is206 = self._process_range_request(environ, complete_length, accept_ranges)
            if not is206 and not is_resource_modified(
                environ,
                self.headers.get("ETag"),
                last_modified=self.headers.get("Last-Modified"),
            ):
                if ETagSet.from_header(environ.get("HTTP_IF_MATCH")):
                    self.status_code = 412
                else:
                    self.status_code = 304
            if (
                self.automatically_set_content_length
                and "Content-Length" not in self.headers
            ):
                length = self.calculate_content_length()
                if length is not None:
                    self.headers["Content-Length"] = str(length)
        return self

