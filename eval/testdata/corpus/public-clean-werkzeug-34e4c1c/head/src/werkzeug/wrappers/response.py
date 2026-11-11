from ..datastructures.headers import Headers

from ..http import generate_etag

from ..http import remove_entity_headers

class Response(_SansIOResponse):
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
                return
        else:
            if (
                request.method in {"GET", "HEAD"}
                and last_modified is not None
                and request.if_modified_since is not None
                and last_modified <= request.if_modified_since
            ):
                self.status_code = 304

    def apply_range(self, request: Request) -> None:
        """Advertise range request support, check :attr:`.Request.if_range`, and
        apply :attr:`.Request.range` if it is valid and satisfiable. The
        response's :attr:`data`, :attr:`content_length`, :attr:`etag`, and
        :attr:`last_modified` must already be set for the complete data.

        Only the ``bytess`` unit is supported. If multiple ranges are given,
        only the first is used. An unknown length is not supported.

        Call :meth:`apply_conditions` before this, to exit early if a condition
        was not met.

        :param request: The request to get the range from.
        :raises .RequestedRangeNotSatisfiable: If the range is valid but cannot
            be satisfied for the content length.

        .. versionadded:: 3.2
        """
        from ..exceptions import RequestedRangeNotSatisfiable

        self.accept_ranges = "bytes"

        if (
            self.status_code != 200
            or request.method != "GET"
            or request.range is None
            or request.range.units != "bytes"
            or not (complete_length := self.content_length)
            or (
                request.if_range
                and not request.if_range.check(self.etag, self.last_modified)
            )
        ):
            return

        if (content_range := request.range.make_content_range(complete_length)) is None:
            raise RequestedRangeNotSatisfiable(complete_length)

        content_length = content_range.stop - content_range.start  # type: ignore[operator]
        self.content_length = content_length
        self.content_range = content_range
        self.status_code = 206
        self.response = _RangeWrapper(
            self.response,  # type: ignore[arg-type]
            content_range.start,  # type: ignore[arg-type]
            content_length,
        )

    def make_conditional(
        self,
        request_or_environ: Request | WSGIEnvironment,
        accept_ranges: bool = False,
        complete_length: int | None = None,
    ) -> Response:
        """Check preconditions from the request, advertise range support, and
        apply a requested range. :attr:`data`, :attr:`content_length`,
        :attr:`etag`, and :attr:`last_modified` must already be set.

        :param request_or_environ: The request or WSGI environ to get the
            preconditions and range from.
        :param accept_ranges: If ``True``, after checking preconditions,
            advertise range support and apply a requested range.
        :param complete_length: Set :attr:`content_length` to this value. Better
            to set it before calling instead.
        :raises .PreconditionFailed: If a precondition fails and results in a
            412 instead of a 304.
        :raises .RequestedRangeNotSatisfiable: If the range is valid but cannot
            be satisfied for the content length.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 3.3. Use ``apply_conditions`` and
            ``apply_range`` instead.

        .. versionchanged:: 3.2
            Applies to all request methods correctly.

            Does not set ``Date``. Does not calculate ``Content-Length``
            if not given.

            Correct ordering of preconditions followed by range.

            Only the ``bytes`` unit is allowed. Sets ``Accept-Ranges``
            even if the range is not valid or satisfiable.

            Raises ``PreconditionFailed`` instead of setting a ``412`` status.

        .. versionchanged:: 2.0
            Range processing is skipped if length is 0 instead of raising a 416.
        """
        import warnings

        warnings.warn(
            "'make_conditional' is deprecated and will be removed in Werkzeug 3.3."
            " Use 'apply_conditions' and 'apply_range' instead.",
            DeprecationWarning,
            stacklevel=2,
        )
        from .request import Request

        if isinstance(request_or_environ, Request):
            request = request_or_environ
        else:
            request = Request(request_or_environ)

        self.apply_conditions(request)

        if accept_ranges:
            if complete_length is not None:
                self.content_length = complete_length

            self.apply_range(request)

        return self
