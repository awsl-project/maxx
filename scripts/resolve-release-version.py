#!/usr/bin/env python3
"""Resolve a stable vMAJOR.MINOR.PATCH release from manual input or Git tags."""
import re
import subprocess
import sys

PATTERN = re.compile(r"v(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)")


def resolve_version(manual, tags):
    manual = "".join(manual.split())
    if manual:
        if not PATTERN.fullmatch(manual):
            raise ValueError("version must be vMAJOR.MINOR.PATCH without leading zeroes")
        return manual
    versions = [tuple(map(int, match.groups())) for tag in tags
                if (match := PATTERN.fullmatch(tag.strip()))]
    if not versions:
        return "v0.0.1"
    major, minor, patch = max(versions)
    return f"v{major}.{minor}.{patch + 1}"


def main():
    if len(sys.argv) > 2:
        raise ValueError("usage: resolve-release-version.py [version]")
    manual = sys.argv[1] if len(sys.argv) == 2 else ""
    # Manual releases do not require reading Git tags.
    tags = [] if manual.strip() else subprocess.check_output(
        ["git", "tag", "--list"], text=True).splitlines()
    print(resolve_version(manual, tags))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
