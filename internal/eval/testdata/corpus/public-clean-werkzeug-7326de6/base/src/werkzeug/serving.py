class WSGIRequestHandler(BaseHTTPRequestHandler):
    def log(self, type: str, message: str, *args: t.Any) -> None:
        _log(
            type,
            f"{self.address_string()} - - [{self.log_date_time_string()}] {message}\n",
            *args,
        )
