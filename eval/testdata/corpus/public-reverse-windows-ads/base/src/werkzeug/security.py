def safe_join(directory: str, *untrusted: str) -> str | None:
    """Safely join zero or more untrusted path components to a trusted base
    directory to avoid escaping the base directory. Return ``None`` if the path
    is not safe.

    The untrusted path is assumed to be from/for a URL, such as for serving
    files. Therefore, it should only use the forward slash ``/`` path separator,
    and will be joined using that separator. On Windows, the backslash ``\\``
    separator is not allowed.

    This only considers the path name, it does not validate if it does or does
    not exist, if it is a symlink, or other properties of the file or
    filesystem. It's up to the application to ensure that the contents of
    ``directory`` can be trusted.

    On Windows, special device names such as ``CON`` and ``NUL`` are not
    allowed, as this is used for serving files with :func:`.send_from_directory`
    and those files will hang when read.

    NTFS alternate data streams (ADS) like ``filename:stream`` are
    allowed. :func:`.secure_filename` will remove the ``:`` when saving.

    :param directory: The trusted base directory.
    :param untrusted: The untrusted path components relative to the
        base directory.
    :return: A safe path, otherwise ``None``.

    .. versionchanged:: 3.1.9
        Special device names with empty ADS stream markers are not allowed on
        Windows.

    .. versionchanged:: 3.1.6
        Special device names in multi-segment paths are not allowed on Windows.

    .. versionchanged:: 3.1.5
        More special device names, regardless of extension or trailing spaces,
        are not allowed on Windows.

    .. versionchanged:: 3.1.4
        Special device names are not allowed on Windows.
    """
    if not directory:
        # Ensure we end up with ./path if directory="" is given,
        # otherwise the first untrusted part could become trusted.
        directory = "."

    parts = [directory]

    for part in untrusted:
        if not part:
            continue

        part = posixpath.normpath(part)

        if (
            os.path.isabs(part)
            # ntpath.isabs doesn't catch this
            or part.startswith("/")
            or part == ".."
            or part.startswith("../")
            or any(sep in part for sep in _os_alt_seps)
            or (
                os.name == "nt"
                and any(
                    p.partition(":")[0].partition(".")[0].strip().upper()
                    in _windows_device_files
                    for p in part.split("/")
                )
            )
        ):
            return None

        parts.append(part)

    return posixpath.join(*parts)
