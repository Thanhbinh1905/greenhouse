class Response(_SansIOResponse):
    def get_wsgi_headers(self, environ: WSGIEnvironment) -> Headers:
        """This is automatically called right before the response is started
        and returns headers modified for the given environment.  It returns a
        copy of the headers from the response with some modifications applied
        if necessary.

        For example the location header (if present) is joined with the root
        URL of the environment.  Also the content length is automatically set
        to zero here for certain status codes.

        .. versionchanged:: 0.6
           Previously that function was called `fix_headers` and modified
           the response object in place.  Also since 0.6, IRIs in location
           and content-location headers are handled properly.

           Also starting with 0.6, Werkzeug will attempt to set the content
           length if it is able to figure it out on its own.  This is the
           case if all the strings in the response iterable are already
           encoded and the iterable is buffered.

        :param environ: the WSGI environment of the request.
        :return: returns a new :class:`~werkzeug.datastructures.Headers`
                 object.
        """
        headers = Headers(self.headers)
        location: str | None = None
        content_location: str | None = None
        content_length: str | int | None = None
        status = self.status_code

        # iterate over the headers to find all values in one go.  Because
        # get_wsgi_headers is used each response that gives us a tiny
        # speedup.
        for key, value in headers:
            ikey = key.lower()
            if ikey == "location":
                location = value
            elif ikey == "content-location":
                content_location = value
            elif ikey == "content-length":
                content_length = value

        if location is not None:
            location = iri_to_uri(location)

            if self.autocorrect_location_header:
                import warnings

                from .request import Request

                warnings.warn(
                    "Setting 'Response.autocorrect_location_header' is deprecated"
                    " and will be removed in Werkzeug 4.0. Set 'response.location'"
                    " directly if you need a specific format.",
                    DeprecationWarning,
                    stacklevel=2,
                )
                # Make the location header an absolute URL.
                current_url = iri_to_uri(Request(environ).base_url)
                location = urljoin(current_url, location)

            headers["Location"] = location

        # make sure the content location is a URL
        if content_location is not None:
            headers["Content-Location"] = iri_to_uri(content_location)

        if 100 <= status < 200 or status == 204:
            # Per section 3.3.2 of RFC 7230, "a server MUST NOT send a
            # Content-Length header field in any response with a status
            # code of 1xx (Informational) or 204 (No Content)."
            headers.remove("Content-Length")
        elif status == 304:
            # remove representation metadata
            # https://httpwg.org/specs/rfc9110.html#status.304
            # https://httpwg.org/specs/rfc9110.html#representation.metadata
            headers = Headers((k, v) for k, v in headers.items(lower=True) if k in {})

        # if we can determine the content length automatically, we
        # should try to do that.  But only if this does not involve
        # flattening the iterator or encoding of strings in the
        # response. We however should not do that if we have a 304
        # response.
        if (
            self.automatically_set_content_length
            and self.is_sequence
            and content_length is None
            and status not in (204, 304)
            and not (100 <= status < 200)
        ):
            content_length = sum(len(x) for x in self.iter_encoded())
            headers["Content-Length"] = str(content_length)

        return headers
