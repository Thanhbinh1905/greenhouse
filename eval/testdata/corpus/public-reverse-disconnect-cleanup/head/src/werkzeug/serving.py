import datetime as dt

class WSGIRequestHandler(BaseHTTPRequestHandler):
    def run_wsgi(self) -> None:
        self.environ = environ = self.make_environ()
        status_set: str | None = None
        headers_set: list[tuple[str, str]] | None = None
        status_sent: str | None = None
        headers_sent: list[tuple[str, str]] | None = None
        chunk_response: bool = False

        def write(data: bytes) -> None:
            nonlocal status_sent, headers_sent, chunk_response
            assert status_set is not None, "write() before start_response"
            assert headers_set is not None, "write() before start_response"
            if status_sent is None:
                status_sent = status_set
                headers_sent = headers_set
                try:
                    code_str, msg = status_sent.split(None, 1)
                except ValueError:
                    code_str, msg = status_sent, ""
                code = int(code_str)
                self.send_response(code, msg)
                header_keys = set()
                for key, value in headers_sent:
                    self.send_header(key, value)
                    header_keys.add(key.lower())

                # Use chunked transfer encoding if there is no content
                # length. Do not use for 1xx and 204 responses. 304
                # responses and HEAD requests are also excluded, which
                # is the more conservative behavior and matches other
                # parts of the code.
                # https://httpwg.org/specs/rfc7230.html#rfc.section.3.3.1
                if (
                    not (
                        "content-length" in header_keys
                        or environ["REQUEST_METHOD"] == "HEAD"
                        or (100 <= code < 200)
                        or code in {204, 304}
                    )
                    and self.protocol_version >= "HTTP/1.1"
                ):
                    chunk_response = True
                    self.send_header("Transfer-Encoding", "chunked")

                # Always close the connection. This disables HTTP/1.1
                # keep-alive connections. They aren't handled well by
                # Python's http.server because it doesn't know how to
                # drain the stream before the next request line.
                self.send_header("Connection", "close")
                self.end_headers()

            assert isinstance(data, bytes), "applications must write bytes"

            if data:
                if chunk_response:
                    self.wfile.write(hex(len(data))[2:].encode())
                    self.wfile.write(b"\r\n")

                self.wfile.write(data)

                if chunk_response:
                    self.wfile.write(b"\r\n")

            self.wfile.flush()

        def start_response(
            status: str,
            headers: list[tuple[str, str]],
            exc_info: tuple[type[BaseException], BaseException, TracebackType]
            | tuple[None, None, None]
            | None = None,
        ) -> t.Callable[[bytes], object]:
            nonlocal status_set, headers_set
            if exc_info:
                try:
                    if headers_sent:
                        raise exc_info[1].with_traceback(exc_info[2])  # type: ignore[union-attr]
                finally:
                    exc_info = None
            elif headers_set:
                raise AssertionError("Headers already set")
            status_set = status
            headers_set = headers
            return write

        def execute(app: WSGIApplication) -> None:
            application_iter = app(environ, start_response)
            try:
                for data in application_iter:
                    write(data)
                if not headers_sent:
                    write(b"")
                if chunk_response:
                    self.wfile.write(b"0\r\n\r\n")
            finally:
                # Check for any remaining data in the read socket, and discard it. This
                # will read past request.max_content_length, but lets the client see a
                # 413 response instead of a connection reset failure. If we supported
                # keep-alive connections, this naive approach would break by reading the
                # next request line. Since we know that write (above) closes every
                # connection we can read everything.
                selector = selectors.DefaultSelector()
                selector.register(self.connection, selectors.EVENT_READ)
                total_size = 0
                total_reads = 0

                # A timeout of 0 tends to fail because a client needs a small amount of
                # time to continue sending its data.
                while selector.select(timeout=0.01):
                    # Only read 10MB into memory at a time.
                    data = self.rfile.read(10_000_000)
                    total_size += len(data)
                    total_reads += 1

                    # Stop reading on no data, >=10GB, or 1000 reads. If a client sends
                    # more than that, they'll get a connection reset failure.
                    if not data or total_size >= 10_000_000_000 or total_reads > 1000:
                        break

                selector.close()

                if hasattr(application_iter, "close"):
                    application_iter.close()

        try:
            execute(self.server.app)
        except connection_dropped_errors as e:
            self.connection_dropped(e, environ)
        except Exception as e:
            if self.server.passthrough_errors:
                raise

            if status_sent is not None and chunk_response:
                self.close_connection = True

            try:
                # if we haven't yet sent the headers but they are set
                # we roll back to be able to set them again.
                if status_sent is None:
                    status_set = None
                    headers_set = None
                execute(InternalServerError())
            except Exception:
                pass

            from .debug.tbtools import DebugTraceback

            msg = DebugTraceback(e).render_traceback_text()
            self.server.log("error", f"Error on request:\n{msg}")
