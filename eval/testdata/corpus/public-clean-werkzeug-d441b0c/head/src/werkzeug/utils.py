from .datastructures.cache_control import ResponseCacheControl

from .datastructures.etag import ETag

def send_file(
    path_or_file: os.PathLike[str] | str | t.IO[bytes],
    environ: WSGIEnvironment,
    mimetype: str | None = None,
    as_attachment: bool = False,
    download_name: str | None = None,
    conditional: bool = True,
    etag: bool | ETag | str = True,
    last_modified: datetime | int | float | None = None,
    max_age: int
    | t.Callable[[os.PathLike[str] | str | None], int | None]
    | None = None,
    use_x_sendfile: bool = False,
    response_class: type[Response] | None = None,
    _root_path: os.PathLike[str] | str | None = None,
) -> Response:
    """Send the contents of a file to the client.

    The first argument can be a file path or a file-like object in ``rb`` mode.
    Paths are preferred in most cases because Werkzeug can manage the file.
    Passing a file-like object is mostly useful when building a file in memory
    with :class:`io.BytesIO`. If the file is seekable, its position will be set
    to the beginning.

    Never pass a path provided by a user. The path is assumed to be trusted, so
    a user could craft a path to access a file you didn't intend. Use
    :func:`send_from_directory` to safely serve user-provided paths.

    If the WSGI server provides ``environ["wsgi.file_wrapper"]``, it is
    used. Alternatively, if the HTTP server supports ``X-Sendfile``,
    ``use_x_sendfile=True`` will tell the server to send the given path, which
    is much more efficient than reading it in Python.

    :param path_or_file: The path to the file to send, relative to the current
        directory if a relative path is given. Alternatively, a file-like object
        in ``rb`` mode. The path will be ``file.name`` if available.
    :param environ: The WSGI environ for the current request.
    :param mimetype: The MIME type to send for the file. If not provided, it is
        detected from the file, and falls back to ``application/octet-stream``.
    :param as_attachment: Indicate to a browser that it should offer to save the
        file instead of displaying it.
    :param download_name: The default name browsers will use when saving the
        file. Defaults to the passed file name.
    :param conditional: Enable conditional and range responses based on request
        headers.
    :param etag: Generate a strong ETag based on the file's last modified time
        and size if available. Or an already generated ETag, assumed strong if a
        ``str`` is given.
    :param last_modified: The last modified time as a ``datetime`` or seconds
        seconds. If not provided, it is detected from the file. Typically only
        needs to be provided for ``BytesIO``.
    :param max_age: How long the client should cache the file, in seconds. If
        set, ``Cache-Control`` will be ``public``, otherwise it will be
        ``no-cache`` to prefer conditional caching.
    :param use_x_sendfile: Set the ``X-Sendfile`` header to let the server
        efficiently send the file. Requires support from the HTTP server..
    :param response_class: Build the response using this class. Defaults to
        :class:`.Response`.
    :param _root_path: Do not use. For internal use only. Use
        :func:`send_from_directory` to safely send files under a path.

    .. versionchanged:: 3.2
        Improve handling of conditional and range requests.

        Path, size, and modification time are detected from file-like objects.
        The position is set to the beginning.

        If ``mimetype`` nor ``download_name`` is given, default to
        ``application/octet-stream``.

    .. versionchanged:: 2.0.2
        ``send_file`` only sets a detected ``Content-Encoding`` if
        ``as_attachment`` is disabled.

    .. versionadded:: 2.0
        Adapted from Flask's implementation.

        ``download_name`` replaces Flask's ``attachment_filename`` parameter. If
        ``as_attachment=False``, it is passed with
        ``Content-Disposition: inline`` instead.

        ``max_age`` replaces Flask's ``cache_timeout`` parameter.
        ``conditional`` is enabled and ``max_age`` is not set by default.

        ``etag`` replaces Flask's ``add_etags`` parameter. It can be a string to
        use instead of generating one.

        If an encoding is returned when guessing ``mimetype``, set the
        ``Content-Encoding`` header.
    """
    if response_class is None:
        from .wrappers import Response

        response_class = Response

    path: os.PathLike[str] | str | None = None
    file: t.IO[bytes]
    size: int | None = None
    headers = Headers()

    if isinstance(path_or_file, (os.PathLike, str)):
        path = path_or_file

        # Flask will pass app.root_path, allowing its send_file wrapper
        # to not have to deal with paths.
        if _root_path is not None:
            path = os.path.join(_root_path, path)
        else:
            path = os.path.abspath(path)

        stat = os.stat(path)
        size = stat.st_size

        if last_modified is None:
            last_modified = stat.st_mtime

        file = open(path, "rb")
    else:
        file = path_or_file

        if isinstance(file, io.TextIOBase):
            raise ValueError("File must be in binary mode, or 'BytesIO'.")

        if hasattr(file, "name"):
            path = file.name

        if file.seekable():
            size = file.seek(0, os.SEEK_END)
            file.seek(0)

        if size is None or last_modified is None:
            try:
                fileno = file.fileno()
            except OSError:
                pass
            else:
                stat = os.stat(fileno)

                if size is None:
                    size = stat.st_size

                if last_modified is None:
                    last_modified = stat.st_mtime

    if download_name is None and path is not None:
        download_name = os.path.basename(path)

    if mimetype is None:
        if download_name is None:
            mimetype = "application/octet-stream"
        else:
            mimetype, encoding = mimetypes.guess_type(download_name)

            if mimetype is None:
                mimetype = "application/octet-stream"

            # Don't send encoding for attachments, it causes browsers to
            # decompress tar.gz files when saving.
            if encoding is not None and not as_attachment:
                headers["Content-Encoding"] = encoding

    if download_name is not None:
        if download_name.isascii():
            names = {"filename": download_name}
        else:
            simple = unicodedata.normalize("NFKD", download_name)
            simple = simple.encode("ascii", "ignore").decode("ascii")
            # safe = RFC 5987 attr-char
            quoted = quote(download_name, safe="!#$&+-.^_`|~")
            names = {"filename": simple, "filename*": f"UTF-8''{quoted}"}

        value = "attachment" if as_attachment else "inline"
        headers.set("Content-Disposition", value, **names)
    elif as_attachment:
        raise TypeError(
            "No name provided for attachment. Either pass 'download_name', or"
            " pass a path instead of a file."
        )

    rv = response_class(
        wrap_file(environ, file),
        mimetype=mimetype,
        headers=headers,
        direct_passthrough=True,
    )
    # Always call file.close, wsgi.file_wrapper does not require a close method.
    rv.call_on_close(file.close)

    if use_x_sendfile and path is not None:
        rv.headers["X-Sendfile"] = path
        rv.response = []

    if size is not None:
        rv.content_length = size

    if last_modified is not None:
        rv.last_modified = last_modified

    cache_control = ResponseCacheControl()
    cache_control.no_cache = True

    # Flask will pass app.get_send_file_max_age, allowing its send_file
    # wrapper to not have to deal with paths.
    if callable(max_age):
        max_age = max_age(path)

    if max_age is not None:
        if max_age > 0:
            cache_control.no_cache = None
            cache_control.public = True

        cache_control.max_age = max_age
        rv.expires = int(time() + max_age)

    rv.cache_control = cache_control

    if isinstance(etag, str):
        rv.etag = etag
    elif etag:
        parts = []

        if last_modified is not None:
            if isinstance(last_modified, datetime):
                parts.append(str(last_modified.timestamp()))
            else:
                parts.append(str(last_modified))

        if size is not None:
            parts.append(str(size))

        rv.etag = "-".join(parts)

    if conditional:
        from .wrappers.request import Request

        request = Request(environ)

        try:
            rv.apply_conditions(request)
            rv.apply_range(request)
        except HTTPException:
            rv.close()
            raise

        if rv.status_code == 304:
            rv.headers.pop("X-Sendfile", None)

    return rv

