class Request(_SansIORequest):
    @classmethod
    def application(
        cls, f: t.Callable[..., wt.WSGIApplication]
    ) -> t.Callable[..., t.Iterable[bytes]]:
        """Create a WSGI application by decorating a function that takes a
        :class:`Request` and returns a :class:`.Response`. If the function
        raises an :exc:`.HTTPException`, it will be converted to an error
        response. :meth:`Request.close` is called when returning.

        .. code-block:: python

            @Request.application
            def my_wsgi_app(request: Request) -> Response:
                return Response("Hello, World!")

        If a request was already created with ``populate_request=True`` (the
        default) it will be taken from``environ["werkzeug.request"]`` instead of
        creating a new object.

        :param f: The application callable.

        .. versionchanged:: 0.14
            ``HTTPException`` is caught and converted to a response.
        """
        from ..exceptions import HTTPException

        @functools.wraps(f)
        def application(*args: t.Any) -> cabc.Iterable[bytes]:
            # Allow either a function (environ, start_response) or a
            # bound method (self, environ, start_response).
            *rest, environ, start_response = args

            with cls(environ) as request:
                try:
                    response = f(*rest, request)
                except HTTPException as e:
                    response = e.get_response(request)

                return response(request.environ, start_response)

        return application

