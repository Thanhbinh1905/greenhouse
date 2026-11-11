class MisdirectedRequest(HTTPException):
    """421 Misdirected Request

    Indicates that the request was directed to a server that is not able to
    produce a response.
    """

    code = 421
    description = "The server is not able to produce a response."
