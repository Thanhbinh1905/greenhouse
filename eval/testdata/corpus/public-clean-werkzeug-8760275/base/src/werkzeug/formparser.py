class MultiPartParser:
    def __init__(
        self,
        stream_factory: TStreamFactory | None = None,
        max_form_memory_size: int | None = None,
        cls: type[MultiDict[str, t.Any]] | None = None,
        buffer_size: int = 64 * 1024,
        max_form_parts: int | None = None,
    ) -> None:
        self.max_form_memory_size = max_form_memory_size
        self.max_form_parts = max_form_parts

        if stream_factory is None:
            stream_factory = default_stream_factory

        self.stream_factory = stream_factory

        if cls is None:
            cls = t.cast("type[MultiDict[str, t.Any]]", MultiDict)

        self.cls = cls
        self.buffer_size = buffer_size

    def fail(self, message: str) -> te.NoReturn:
        raise ValueError(message)

    def get_part_charset(self, headers: Headers) -> str:
        # Figure out input charset for current part
        content_type = headers.get("content-type")

        if content_type:
            parameters = parse_options_header(content_type)[1]
            ct_charset = parameters.get("charset", "").lower()

            # A safe list of encodings. Modern clients should only send ASCII or UTF-8.
            # This list will not be extended further.
            if ct_charset in {"ascii", "us-ascii", "utf-8", "iso-8859-1"}:
                return ct_charset

        return "utf-8"

    def start_file_streaming(
        self, event: File, total_content_length: int | None
    ) -> t.IO[bytes]:
        content_type = event.headers.get("content-type")

        try:
            content_length = _plain_int(event.headers["content-length"])
        except (KeyError, ValueError):
            content_length = 0

        container = self.stream_factory(
            total_content_length=total_content_length,
            filename=event.filename,
            content_type=content_type,
            content_length=content_length,
        )
        return container

    def parse(
        self, stream: t.IO[bytes], boundary: bytes, content_length: int | None
    ) -> tuple[MultiDict[str, str], MultiDict[str, FileStorage]]:
        current_part: Field | File
        container: t.IO[bytes] | list[bytes]
        _write: t.Callable[[bytes], t.Any]

        parser = MultipartDecoder(
            boundary,
            max_form_memory_size=self.max_form_memory_size,
            max_parts=self.max_form_parts,
        )

        fields = []
        files = []

        for data in _chunk_iter(stream.read, self.buffer_size):
            parser.receive_data(data)
            event = parser.next_event()
            while not isinstance(event, (Epilogue, NeedData)):
                if isinstance(event, Field):
                    current_part = event
                    container = []
                    _write = container.append
                elif isinstance(event, File):
                    current_part = event
                    container = self.start_file_streaming(event, content_length)
                    _write = container.write
                elif isinstance(event, Data):
                    _write(event.data)
                    if not event.more_data:
                        if isinstance(current_part, Field):
                            value = b"".join(container).decode(
                                self.get_part_charset(current_part.headers), "replace"
                            )
                            fields.append((current_part.name, value))
                        else:
                            container = t.cast(t.IO[bytes], container)
                            container.seek(0)
                            files.append(
                                (
                                    current_part.name,
                                    FileStorage(
                                        container,
                                        current_part.filename,
                                        current_part.name,
                                        headers=current_part.headers,
                                    ),
                                )
                            )

                event = parser.next_event()

        return self.cls(fields), self.cls(files)
