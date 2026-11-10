_mime_split_re = re.compile(r"/|(?:[ \t]*;[ \t]*)")

def _normalize_mime(value: str) -> list[str]:
    return _mime_split_re.split(value.lower())

class MIMEAccept(Accept):
    """A parsed ``Accept`` header."""

    def _specificity(self, value: str) -> tuple[bool, ...]:
        return tuple(x != "*" for x in _mime_split_re.split(value))

    def _value_matches(self, app_value: str, req_value: str) -> bool:
        # from the client, can't match
        if "/" not in req_value:
            return False

        req_type, req_sub, *req_params = _normalize_mime(req_value)
        req_params.sort()

        # */* is only value that starts with *
        if req_type == "*" and req_sub != "*":
            return False

        # from the application, tell the developer it's not valid
        if "/" not in app_value:
            raise ValueError(f"invalid mimetype {app_value!r}")

        app_type, app_sub, *app_params = _normalize_mime(app_value)
        app_params.sort()

        if app_type == "*" and app_sub != "*":
            raise ValueError(f"invalid mimetype {app_value!r}")

        return (
            (req_type == "*" and req_sub == "*")
            or (app_type == "*" and app_sub == "*")
            or (
                req_type == app_type
                and (
                    req_sub == "*"
                    or app_sub == "*"
                    or (req_sub == app_sub and req_params == app_params)
                )
            )
        )

    @property
    def accept_html(self) -> bool:
        """True if ``text/html`` is accepted.

        .. versionchanged:: 3.2
            Does not match XHTML or XML.
        """
        return "text/html" in self  # type: ignore[comparison-overlap]

    @property
    def accept_xhtml(self) -> bool:
        """True if ``application/xhtml+xml`` or ``application/xml`` is accepted.

        .. deprecated:: 3.2
            Will be removed in Werkzeug 4.0. Use ``in`` or ``best_match`` instead.
        """
        import warnings

        warnings.warn(
            "'accept_xhtml' is deprecated and will be removed in Werkzeug 4.0. Use 'in'"
            " or 'best_match' instead.",
            DeprecationWarning,
            stacklevel=2,
        )
        return "application/xhtml+xml" in self or "application/xml" in self  # type: ignore[comparison-overlap]

    @property
    def accept_json(self) -> bool:
        """True if ``application/json`` is accepted."""
        return "application/json" in self  # type: ignore[comparison-overlap]
