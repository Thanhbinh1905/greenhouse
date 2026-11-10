import typing as t

class Response:
    @www_authenticate.register_setter
    def _set_www_authenticate(
        self, value: WWWAuthenticate | list[WWWAuthenticate]
    ) -> None:
        if isinstance(value, list):
            self.headers.setlist("WWW-Authenticate", (v.to_header() for v in value))
        else:
            self.headers["WWW-Authenticate"] = value.to_header()
            value._on_update = make_structure_on_update(
                self, "WWW-Authenticate", WWWAuthenticate
            )
