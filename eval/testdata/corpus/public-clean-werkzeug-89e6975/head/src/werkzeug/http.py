_cookie_re = re.compile(
    r"""
    [ \t]*+  # ignore leading space
    ([^ \t=";]++)  # key
    (?:[ \t]*+=[ \t]*+  # optional =value, ignoring invalid space
        (
            "(?:[^\\"]|\\.)*+"  # quoted value with backslash escapes
        |
            [^ \t";]*+  # token value
        )
    )?
    [ \t]*+  # ignore trailing space
    (?:;|\Z)  # only if followed by semicolon or end
    """,
    flags=re.ASCII | re.VERBOSE,
)
