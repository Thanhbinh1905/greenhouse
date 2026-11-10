class EnvironBuilder:
    def __init__(
        self,
        path: str = "/",
        base_url: str | None = None,
        query_string: t.Mapping[str, str] | str | None = None,
        method: str = "GET",
        input_stream: t.IO[bytes] | None = None,
        content_type: str | None = None,
        content_length: int | None = None,
        errors_stream: t.IO[str] | None = None,
        multithread: bool = False,
        multiprocess: bool = False,
        run_once: bool = False,
        headers: Headers
        | t.Mapping[str, str]
        | t.Iterable[tuple[str, str]]
        | None = None,
        data: t.IO[bytes] | str | bytes | t.Mapping[str, t.Any] | None = None,
        environ_base: t.Mapping[str, t.Any] | None = None,
        environ_overrides: t.Mapping[str, t.Any] | None = None,
        mimetype: str | None = None,
        json: t.Mapping[str, t.Any] | None = None,
        auth: Authorization | tuple[str, str] | None = None,
    ) -> None:
        if query_string is not None and "?" in path:
            raise ValueError("Query string is defined in the path and as an argument")
        request_uri = urlsplit(path)
        if query_string is None and "?" in path:
            query_string = request_uri.query

        self.path = iri_to_uri(request_uri.path)
        self.request_uri = path
        if base_url is not None:
            base_url = iri_to_uri(base_url)
        self.base_url = base_url
        if isinstance(query_string, str):
            self.query_string = query_string
        else:
            if query_string is None:
                query_string = MultiDict()
            elif not isinstance(query_string, MultiDict):
                query_string = MultiDict(query_string)
            self.args = query_string
        self.method = method
        if headers is None:
            headers = Headers()
        elif not isinstance(headers, Headers):
            headers = Headers(headers)
        self.headers = headers
        if content_type is not None:
            self.content_type = content_type
        if errors_stream is None:
            errors_stream = sys.stderr
        self.errors_stream = errors_stream
        self.multithread = multithread
        self.multiprocess = multiprocess
        self.run_once = run_once
        self.environ_base = environ_base
        self.environ_overrides = environ_overrides
        self.input_stream = input_stream
        self.content_length = content_length

        if auth is not None:
            if isinstance(auth, tuple):
                auth = Authorization(
                    "basic", {"username": auth[0], "password": auth[1]}
                )

            self.headers.set("Authorization", auth.to_header())

        if json is not None:
            if data is not None:
                raise TypeError("can't provide both json and data")

            data = self.json_dumps(json)

            if self.content_type is None:
                self.content_type = "application/json"

        if data:
            if input_stream is not None:
                raise TypeError("can't provide input stream and data")
            if hasattr(data, "read"):
                data = data.read()
            if isinstance(data, str):
                data = data.encode()
            if isinstance(data, bytes):
                self.input_stream = BytesIO(data)
                if self.content_length is None:
                    self.content_length = len(data)
            else:
                for key, value in _iter_data(data):
                    if isinstance(value, (tuple, dict)) or hasattr(value, "read"):
                        self._add_file_from_data(key, value)
                    else:
                        self.form.setlistdefault(key).append(value)

        if mimetype is not None:
            self.mimetype = mimetype

