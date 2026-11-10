def parse_dict_header(value: str) -> dict[str, str | None]:
    """Parse a list header using :func:`parse_list_header`, then parse each item as a
    ``key=value`` pair.

    .. code-block:: python

        parse_dict_header('a=b, c="d, e", f')
        {"a": "b", "c": "d, e", "f": None}

    This is the reverse of :func:`dump_header`.

    If a key does not have a value, it is ``None``.

    This handles charsets for values as described in
    `RFC 2231 <https://www.rfc-editor.org/rfc/rfc2231#section-3>`__. Only ASCII, UTF-8,
    and ISO-8859-1 charsets are accepted, otherwise the value remains quoted.

    :param value: The header value to parse.

    .. versionchanged:: 3.0
        Passing bytes is not supported.

    .. versionchanged:: 3.0
        The ``cls`` argument is removed.

    .. versionchanged:: 2.3
        Added support for ``key*=charset''value`` encoded items.

    .. versionchanged:: 0.9
       The ``cls`` argument was added.
    """
    result: dict[str, str | None] = {}

    for item in parse_list_header(value):
        key, has_value, value = item.partition("=")
        key = key.strip()

        if not key:
            # =value is not valid
            continue

        if not has_value:
            result[key] = None
            continue

        value = value.strip()
        encoding: str | None = None

        if key[-1] == "*":
            # key*=charset''value becomes key=value, where value is percent encoded
            # adapted from parse_options_header, without the continuation handling
            key = key[:-1]
            match = _charset_value_re.match(value)

            if match:
                # If there is a charset marker in the value, split it off.
                encoding, value = match.groups()
                encoding = encoding.lower()

            # A safe list of encodings. Modern clients should only send ASCII or UTF-8.
            # This list will not be extended further. An invalid encoding will leave the
            # value quoted.
            if encoding in {"ascii", "us-ascii", "utf-8", "iso-8859-1"}:
                # invalid bytes are replaced during unquoting
                value = unquote(value, encoding=encoding)

        if len(value) >= 2 and value[0] == value[-1] == '"':
            value = value[1:-1]

        result[key] = value

    return result

