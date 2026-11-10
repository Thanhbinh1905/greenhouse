class WSGIRequestHandler(BaseHTTPRequestHandler):
    def log(self, type: str, message: str, *args: t.Any) -> None:
        # an IPv6 scoped address contains "%" which breaks logging
        address_string = self.address_string().replace("%", "%%")
        _log(
            type,
            f"{address_string} - - [{self.log_date_time_string()}] {message}\n",
            *args,
        )

