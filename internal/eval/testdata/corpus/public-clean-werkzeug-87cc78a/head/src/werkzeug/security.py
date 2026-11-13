def safe_join(directory: str, *pathnames: str) -> str | None:
    """Safely join zero or more untrusted path components to a base
    directory to avoid escaping the base directory.

    :param directory: The trusted base directory.
    :param pathnames: The untrusted path components relative to the
        base directory.
    :return: A safe path, otherwise ``None``.
    """
    if not directory:
        # Ensure we end up with ./path if directory="" is given,
        # otherwise the first untrusted part could become trusted.
        directory = "."

    parts = [directory]

    for filename in pathnames:
        if filename != "":
            filename = posixpath.normpath(filename)

        if (
            any(sep in filename for sep in _os_alt_seps)
            or os.path.isabs(filename)
            # ntpath.isabs doesn't catch this on Python < 3.11
            or filename.startswith("/")
            or filename == ".."
            or filename.startswith("../")
        ):
            return None

        parts.append(filename)

    return posixpath.join(*parts)
