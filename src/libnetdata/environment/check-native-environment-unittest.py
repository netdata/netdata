#!/usr/bin/env python3

"""Unit tests for the native-environment source scanner."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import pathlib
import tempfile
import unittest


CHECKER_PATH = pathlib.Path(__file__).with_name("check-native-environment.py")
SPEC = importlib.util.spec_from_file_location("check_native_environment", CHECKER_PATH)
if not SPEC or not SPEC.loader:
    raise RuntimeError(f"cannot load {CHECKER_PATH}")

CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class MaskingTests(unittest.TestCase):
    def assert_masked_tokens(self, source: str, expected: list[str]) -> None:
        masked = CHECKER.without_comments_and_literals(source)
        self.assertEqual(len(masked), len(source))
        self.assertEqual(
            [index for index, char in enumerate(masked) if char == "\n"],
            [index for index, char in enumerate(source) if char == "\n"],
        )
        self.assertEqual(CHECKER.RAW_TOKEN.findall(masked), expected)

    def test_code_is_preserved(self) -> None:
        self.assert_masked_tokens("getenv(); setenv();", ["getenv", "setenv"])

    def test_line_comment_is_masked(self) -> None:
        self.assert_masked_tokens("// getenv\nsetenv();", ["setenv"])

    def test_block_comment_is_masked(self) -> None:
        self.assert_masked_tokens("/* getenv\n unsetenv */\nputenv();", ["putenv"])

    def test_string_and_character_literals_are_masked(self) -> None:
        self.assert_masked_tokens(
            "char *s = \"getenv \\\" setenv\"; char c = '\\''; unsetenv();",
            ["unsetenv"],
        )

    def test_cpp_digit_separators_do_not_mask_following_code(self) -> None:
        self.assert_masked_tokens(
            "auto decimal = 1'000; auto hex = 0xCA'FE; "
            "auto grouped = 0xFFFF'FFFF'FFFF; getenv(); setenv();",
            ["getenv", "setenv"],
        )

    def test_cpp_prefixed_character_literal_is_still_masked(self) -> None:
        self.assert_masked_tokens("char8_t c = u8'a'; getenv();", ["getenv"])

    def test_escaped_newline_in_literal_is_preserved(self) -> None:
        self.assert_masked_tokens('char *s = "getenv\\\nsetenv"; putenv();', ["putenv"])

    def test_cpp_raw_literals_are_masked(self) -> None:
        fixtures = (
            'auto s = R"(getenv " // /*)"; setenv();',
            'auto s = u8R"tag(getenv " // /*\nsetenv)tag"; unsetenv();',
            'auto s = uR"(getenv)"; putenv();',
            'auto s = UR"delimiter1234567(getenv)delimiter1234567"; clearenv();',
            'auto s = LR"x(GetEnvironmentVariableA)x"; setenv();',
        )
        expected = (["setenv"], ["unsetenv"], ["putenv"], ["clearenv"], ["setenv"])
        for source, tokens in zip(fixtures, expected):
            with self.subTest(source=source):
                self.assert_masked_tokens(source, tokens)

    def test_unterminated_cpp_raw_literal_is_masked(self) -> None:
        self.assert_masked_tokens('auto s = R"tag(getenv " //', [])

    def test_unterminated_regions_are_masked(self) -> None:
        self.assert_masked_tokens("/* getenv", [])
        self.assert_masked_tokens('"getenv', [])


class AuditTests(unittest.TestCase):
    def test_source_discovery_filters_and_sorts(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            repository = pathlib.Path(directory)
            source = repository / "src"
            source.mkdir()
            (source / "z.c").write_text("", encoding="utf-8")
            (source / "a.h").write_text("", encoding="utf-8")
            (source / "ignored.py").write_text("", encoding="utf-8")

            paths = [path.name for path in CHECKER._source_files(repository)]
            self.assertEqual(paths, ["a.h", "z.c"])

    def test_per_file_diagnostics_preserve_order(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            repository = pathlib.Path(directory)
            path = repository / "src" / "example.c"
            path.parent.mkdir()
            path.write_text("getenv();\ngetenv();\n", encoding="utf-8")
            expected = {"src/example.c": {"getenv": 1, "setenv": 1}}

            relative, violations = CHECKER._audit_source_file(repository, path, expected)
            self.assertEqual(relative, "src/example.c")
            self.assertEqual(
                violations,
                [
                    "src/example.c:2: unexpected native environment token 'getenv'",
                    "src/example.c: expected 1 audited 'setenv' token(s), found 0",
                ],
            )

    def test_literal_token_paste_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            repository = pathlib.Path(directory)
            path = repository / "src" / "example.c"
            path.parent.mkdir()
            path.write_text(
                "get ## env(\"X\");\nGet ## Environment ## Variable ## A(\"X\", 0, 0);\n",
                encoding="utf-8",
            )

            _, violations = CHECKER._audit_source_file(repository, path, {})
            self.assertEqual(
                violations,
                [
                    "src/example.c:1: unexpected token-pasted native environment token 'getenv'",
                    "src/example.c:2: unexpected token-pasted native environment token "
                    "'GetEnvironmentVariableA'",
                ],
            )

    def test_missing_audited_files_preserve_mapping_order(self) -> None:
        expected = {
            "src/present.c": {"getenv": 1},
            "src/missing.c": {"setenv": 2},
        }
        self.assertEqual(
            CHECKER._missing_audited_file_violations({"src/present.c"}, expected),
            ["src/missing.c: audited source file is missing (2 token(s))"],
        )

    def test_report_contract(self) -> None:
        stdout = io.StringIO()
        stderr = io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            self.assertEqual(CHECKER._report([]), 0)
        self.assertEqual(stdout.getvalue(), "native environment bypass check: PASS\n")
        self.assertEqual(stderr.getvalue(), "")

        stdout = io.StringIO()
        stderr = io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            self.assertEqual(CHECKER._report(["violation"]), 1)
        self.assertEqual(stdout.getvalue(), "")
        self.assertEqual(
            stderr.getvalue(),
            "Native process-environment access is private to audited adapters:\n  violation\n",
        )


if __name__ == "__main__":
    unittest.main()
