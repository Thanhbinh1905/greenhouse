class MultipartDecoder:
    def receive_data(self, data: bytes | None) -> None:
        if data is None:
            self.complete = True
        elif (
            self.max_form_memory_size is not None
            and len(self.buffer) + len(data) > self.max_form_memory_size
        ):
            # Ensure that data within single event does not exceed limit.
            # Also checked across accumulated events in MultiPartParser.
            raise RequestEntityTooLarge()
        else:
            self.buffer.extend(data)

