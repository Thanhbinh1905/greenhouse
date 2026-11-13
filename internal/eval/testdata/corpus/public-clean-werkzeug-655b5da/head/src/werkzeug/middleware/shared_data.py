"""
Serve Shared Static Files
=========================

.. autoclass:: SharedDataMiddleware
    :members: is_allowed
"""

import os

from fnmatch import fnmatch

from ..security import safe_join

from ..utils import send_file

from ..wrappers import Request

if t.TYPE_CHECKING:
    from _typeshed.wsgi import WSGIApplication

class SharedDataMiddleware:
    def __init__(
        self,
        app: WSGIApplication,
        exports: (
            cabc.Mapping[str, str | tuple[str, str]]
            | t.Iterable[tuple[str, str | tuple[str, str]]]
        ),
        disallow: str | None = None,
        cache: None = None,
        cache_timeout: int | None = None,
        fallback_mimetype: None = None,
    ) -> None:
        self.app = app
        self.exports: list[tuple[str, _TLoader]] = []
        self.cache = cache
        self.cache_timeout = cache_timeout
        self.fallback_mimetype = fallback_mimetype

        if cache is not None:
            import warnings

            warnings.warn(
                "The 'cache' parameter is deprecated and will be removed"
                " in Werkzeug 4.0.",
                DeprecationWarning,
                stacklevel=2,
            )

        if fallback_mimetype is not None:
            import warnings

            warnings.warn(
                "The 'fallback_mimetype' parameter is deprecated and will be"
                " removed in Werkzeug 4.0.",
                DeprecationWarning,
                stacklevel=2,
            )

        if isinstance(exports, cabc.Mapping):
            exports = exports.items()

        for key, value in exports:
            if isinstance(value, tuple):
                loader = self.get_package_loader(*value)
            elif isinstance(value, str):
                if os.path.isfile(value):
                    loader = self.get_file_loader(value)
                else:
                    loader = self.get_directory_loader(value)
            else:
                raise TypeError(f"unknown def {value!r}")

            self.exports.append((key, loader))

        if disallow is not None:
            self.is_allowed = lambda x: x is not None and not fnmatch(x, disallow)  # type: ignore[method-assign,assignment]

    @Request.application
    def __call__(self, request: Request) -> WSGIApplication:
        path = request.path

        for search_path, loader in self.exports:
            if search_path == path:
                filename, opener = loader(None)

                if opener is not None:
                    break

            if not search_path.endswith("/"):
                search_path += "/"

            if path.startswith(search_path):
                filename, opener = loader(path[len(search_path) :])

                if opener is not None:
                    break
        else:
            return self.app

        if not self.is_allowed(filename):
            return self.app

        f, mtime, size = opener()
        response = send_file(
            f,
            request.environ,
            download_name=filename,
            conditional=bool(self.cache),
            last_modified=mtime,
            max_age=self.cache_timeout,
        )

        if response.content_length is None:
            response.content_length = size

        if (
            self.fallback_mimetype is not None
            and response.mimetype == "application/octet-stream"
        ):
            response.mimetype = self.fallback_mimetype

        return response
