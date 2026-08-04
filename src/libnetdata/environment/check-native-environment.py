#!/usr/bin/env python3

"""Reject native process-environment access outside audited adapters."""

from __future__ import annotations

import pathlib
import re
import sys
from collections import Counter


EXPECTED_RAW_TOKENS = {
    # The managed subsystem's private platform adapter and its native-storage tests.
    "src/libnetdata/environment/environment.c": {
        "NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1,
        "environ": 3,
        "_NSGetEnviron": 2,
        "GetEnvironmentStringsA": 2,
        "FreeEnvironmentStringsA": 2,
        "GetEnvironmentVariableA": 2,
        "SetEnvironmentVariableA": 7,
        "getenv": 3,
        "setenv": 3,
        "unsetenv": 3,
    },
    "src/libnetdata/environment/environment-unittest.c": {
        "NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1,
        "GetEnvironmentStringsA": 1,
        "FreeEnvironmentStringsA": 1,
        "GetEnvironmentVariableA": 2,
        "SetEnvironmentVariableA": 1,
        "getenv": 9,
        "unsetenv": 1,
    },
    # Compatibility bootstrap before the managed subsystem is initialized.
    "src/libnetdata/os/setenv.c": {
        "NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1,
        "GetEnvironmentVariable": 1,
        "SetEnvironmentVariable": 2,
        "getenv": 1,
        "putenv": 1,
        "setenv": 1,
    },
    "src/libnetdata/os/setenv.h": {"setenv": 1},
    "src/libnetdata/log/nd_log-init.c": {"NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1, "getenv": 2},
    # Audited single-threaded restricted execution helpers.
    "src/collectors/utils/nd-run.c": {"clearenv": 1, "environ": 2, "getenv": 5, "setenv": 1},
    "src/collectors/utils/ndsudo.c": {"getenv": 1, "putenv": 1},
    # Retained but inactive or unsupported source covered by the inventory.
    "src/daemon/config/netdata-conf-ssl.c": {
        "NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1,
        "getenv": 2,
        "setenv": 2,
    },
    "src/exporting/pubsub/pubsub_publish.cc": {"setenv": 1},
    # Vendored third-party debug code.
    "src/libnetdata/libjudy/vendored/JudyL/JudyLGet.c": {"getenv": 1},
    "src/libnetdata/libjudy/vendored/JudyL/j__udyLGet.c": {"getenv": 1},
    "src/libnetdata/environment/native-environment-compiler-check.h": {
        "NETDATA_NATIVE_ENVIRONMENT_ACCESS": 1,
        "getenv": 1,
        "_wgetenv": 1,
        "getenv_s": 1,
        "_wgetenv_s": 1,
        "_dupenv_s": 1,
        "_wdupenv_s": 1,
        "secure_getenv": 1,
        "setenv": 1,
        "unsetenv": 1,
        "putenv": 1,
        "_putenv": 1,
        "_wputenv": 1,
        "_putenv_s": 1,
        "_wputenv_s": 1,
        "clearenv": 1,
        "environ": 1,
        "_environ": 1,
        "__environ": 1,
        "_wenviron": 1,
        "_NSGetEnviron": 1,
        "__p__environ": 1,
        "GetEnvironmentStringsA": 2,
        "GetEnvironmentStringsW": 1,
        "FreeEnvironmentStringsA": 1,
        "FreeEnvironmentStringsW": 1,
        "GetEnvironmentVariableA": 1,
        "GetEnvironmentVariableW": 1,
        "SetEnvironmentVariableA": 1,
        "SetEnvironmentVariableW": 1,
        "ExpandEnvironmentStringsA": 1,
        "ExpandEnvironmentStringsW": 1,
    },
}

SOURCE_SUFFIXES = {".c", ".cc", ".cpp", ".h", ".hh", ".hpp", ".m", ".mm"}
NATIVE_TOKEN_PATTERN = (
    r"getenv|_wgetenv|getenv_s|_wgetenv_s|_dupenv_s|_wdupenv_s|secure_getenv|"
    r"setenv|unsetenv|putenv|_putenv|_wputenv|_putenv_s|_wputenv_s|clearenv|"
    r"environ|_environ|__environ|_wenviron|_NSGetEnviron|__p__environ|"
    r"GetEnvironmentStrings(?:A|W)?|FreeEnvironmentStrings(?:A|W)?|"
    r"GetEnvironmentVariable(?:A|W)?|SetEnvironmentVariable(?:A|W)?|"
    r"ExpandEnvironmentStrings(?:A|W)?"
)
RAW_TOKEN = re.compile(rf"\b(?:{NATIVE_TOKEN_PATTERN}|NETDATA_NATIVE_ENVIRONMENT_ACCESS)\b")
NATIVE_TOKEN = re.compile(rf"^(?:{NATIVE_TOKEN_PATTERN})$")
TOKEN_PASTE = re.compile(r"\b[A-Za-z_]\w*(?:\s*##\s*[A-Za-z_]\w*)+\b")
RAW_LITERAL_OPEN = re.compile(
    r'(?<!\w)(?:u8|u|U|L)?R"(?P<delimiter>[^ ()\\\t\v\f\r\n]{0,16})\('
)

MASK_DELIMITERS = (
    ("//", "line-comment"),
    ("/*", "block-comment"),
    ('"', "string"),
    ("'", "character"),
)
QUOTE_BY_STATE = {"string": '"', "character": "'"}
NUMERIC_TOKEN_CHARS = frozenset("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.")


def _mask_non_newlines(source: str) -> str:
    return "".join("\n" if char == "\n" else " " for char in source)


def _is_cpp_digit_separator(source: str, position: int) -> bool:
    if position == 0 or position + 1 >= len(source):
        return False

    if not source[position - 1].isalnum() or not source[position + 1].isalnum():
        return False

    start = position - 1
    while start > 0 and (source[start - 1] in NUMERIC_TOKEN_CHARS or source[start - 1] == "'"):
        start -= 1

    prefix = source[start:position]
    return prefix[0].isdigit() or (prefix.startswith(".") and len(prefix) > 1 and prefix[1].isdigit())


def _find_character_literal(source: str, index: int) -> int:
    position = source.find("'", index)
    while position >= 0 and _is_cpp_digit_separator(source, position):
        position = source.find("'", position + 1)

    return position


def _consume_code_region(source: str, index: int) -> tuple[str, int, str]:
    matches = []
    for delimiter, state in MASK_DELIMITERS:
        position = _find_character_literal(source, index) if delimiter == "'" else source.find(delimiter, index)
        if position >= 0:
            matches.append((position, delimiter, state, None))

    raw_literal = RAW_LITERAL_OPEN.search(source, index)
    if raw_literal:
        matches.append((raw_literal.start(), "", "raw-literal", raw_literal))

    if not matches:
        return source[index:], len(source), "code"

    position, delimiter, state, raw_literal = min(matches, key=lambda item: item[0])
    if raw_literal:
        terminator = ")" + raw_literal.group("delimiter") + '"'
        end = source.find(terminator, raw_literal.end())
        if end < 0:
            end = len(source)
        else:
            end += len(terminator)

        masked = source[index:position] + _mask_non_newlines(source[position:end])
        return masked, end, "code"

    masked = source[index:position] + " " * len(delimiter)
    return masked, position + len(delimiter), state


def _consume_line_comment(source: str, index: int) -> tuple[str, int, str]:
    newline = source.find("\n", index)
    if newline < 0:
        return " " * (len(source) - index), len(source), "line-comment"

    return " " * (newline - index) + "\n", newline + 1, "code"


def _consume_block_comment(source: str, index: int) -> tuple[str, int, str]:
    end = source.find("*/", index)
    if end < 0:
        return _mask_non_newlines(source[index:]), len(source), "block-comment"

    end += 2
    return _mask_non_newlines(source[index:end]), end, "code"


def _consume_quoted_literal(source: str, index: int, state: str) -> tuple[str, int, str]:
    quote = QUOTE_BY_STATE[state]
    end = index
    while end < len(source):
        char = source[end]
        following = source[end + 1] if end + 1 < len(source) else ""
        if char == "\\" and following:
            end += 2
            continue

        end += 1
        if char == quote:
            return _mask_non_newlines(source[index:end]), end, "code"

    return _mask_non_newlines(source[index:end]), end, state


def without_comments_and_literals(source: str) -> str:
    """Replace C/C++ comments and literals with spaces while retaining newlines."""
    output: list[str] = []
    index = 0
    state = "code"

    while index < len(source):
        if state == "code":
            masked, index, state = _consume_code_region(source, index)
        elif state == "line-comment":
            masked, index, state = _consume_line_comment(source, index)
        elif state == "block-comment":
            masked, index, state = _consume_block_comment(source, index)
        else:
            masked, index, state = _consume_quoted_literal(source, index, state)

        output.append(masked)

    return "".join(output)


def _source_files(repository: pathlib.Path):
    for path in sorted((repository / "src").rglob("*")):
        if path.is_file() and path.suffix in SOURCE_SUFFIXES:
            yield path


def _native_occurrences(source: str) -> tuple[Counter[str], list[tuple[int, str, bool]]]:
    matches = list(RAW_TOKEN.finditer(source))
    actual = Counter(match.group(0) for match in matches)
    occurrences = [(match.start(), match.group(0), False) for match in matches]

    for match in TOKEN_PASTE.finditer(source):
        token = re.sub(r"\s*##\s*", "", match.group(0))
        if NATIVE_TOKEN.fullmatch(token):
            occurrences.append((match.start(), token, True))

    return actual, occurrences


def _audit_source_file(
    repository: pathlib.Path,
    path: pathlib.Path,
    expected_raw_tokens=None,
) -> tuple[str, list[str]]:
    if expected_raw_tokens is None:
        expected_raw_tokens = EXPECTED_RAW_TOKENS

    relative = path.relative_to(repository).as_posix()
    source = without_comments_and_literals(path.read_text(encoding="utf-8", errors="replace"))
    actual, occurrences = _native_occurrences(source)
    expected = Counter(expected_raw_tokens.get(relative, {}))
    violations: list[str] = []

    permitted_so_far: Counter[str] = Counter()
    for position, token, pasted in sorted(occurrences):
        line = source.count("\n", 0, position) + 1
        if pasted:
            violations.append(
                f"{relative}:{line}: unexpected token-pasted native environment token '{token}'"
            )
            continue

        permitted_so_far[token] += 1
        if permitted_so_far[token] <= expected[token]:
            continue

        violations.append(f"{relative}:{line}: unexpected native environment token '{token}'")

    for token, count in expected.items():
        if actual[token] < count:
            violations.append(
                f"{relative}: expected {count} audited '{token}' token(s), found {actual[token]}"
            )

    return relative, violations


def _missing_audited_file_violations(
    seen: set[str], expected_raw_tokens=None
) -> list[str]:
    if expected_raw_tokens is None:
        expected_raw_tokens = EXPECTED_RAW_TOKENS

    return [
        f"{relative}: audited source file is missing ({sum(expected.values())} token(s))"
        for relative, expected in expected_raw_tokens.items()
        if relative not in seen
    ]


def _report(violations: list[str]) -> int:

    if violations:
        print("Native process-environment access is private to audited adapters:", file=sys.stderr)
        for violation in violations:
            print(f"  {violation}", file=sys.stderr)
        return 1

    print("native environment bypass check: PASS")
    return 0


def main() -> int:
    """Audit the repository and return a process-compatible status."""
    repository = pathlib.Path(__file__).resolve().parents[3]
    violations: list[str] = []
    seen: set[str] = set()

    for path in _source_files(repository):
        relative, source_violations = _audit_source_file(repository, path)
        seen.add(relative)
        violations.extend(source_violations)

    violations.extend(_missing_audited_file_violations(seen))
    return _report(violations)


if __name__ == "__main__":
    raise SystemExit(main())
