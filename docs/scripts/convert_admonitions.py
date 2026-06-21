#!/usr/bin/env python3
"""Convert GitHub-style markdown admonitions to VitePress custom containers.

Reads from stdin and writes to stdout.
"""

import re
import sys

ADMONITION_START = re.compile(
    r"^>\s+\[!(WARNING|NOTE|TIP|IMPORTANT|CAUTION)\]\s*$",
    re.IGNORECASE,
)

TYPE_MAP = {
    "WARNING": "warning",
    "NOTE": "info",
    "TIP": "tip",
    "IMPORTANT": "details",
    "CAUTION": "danger",
}


def convert_admonitions(lines):
    out = []
    i = 0
    n = len(lines)
    while i < n:
        match = ADMONITION_START.match(lines[i])
        if match:
            kind = match.group(1).upper()
            out.append(f"::: {TYPE_MAP[kind]}")
            i += 1
            while i < n and lines[i].startswith("> "):
                out.append(lines[i][2:])
                i += 1
            out.append(":::")
        else:
            out.append(lines[i])
            i += 1
    return out


def main():
    text = sys.stdin.read()
    lines = text.splitlines()
    converted = convert_admonitions(lines)
    sys.stdout.write("\n".join(converted))
    if text.endswith("\n"):
        sys.stdout.write("\n")


if __name__ == "__main__":
    main()
