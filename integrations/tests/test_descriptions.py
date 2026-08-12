#!/usr/bin/env python3

import ast
import copy
import json
import random
import re
import subprocess
import sys
import tempfile
import unicodedata
import unittest
from pathlib import Path

from jsonschema import ValidationError
from markdown_it import MarkdownIt
from ruamel.yaml import YAML

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = INTEGRATIONS_DIR.parent
sys.path.insert(0, str(INTEGRATIONS_DIR))

from descriptions import (
    DOCUMENTATION_TYPES,
    MAX_DESCRIPTION_LENGTH,
    MIN_DESCRIPTION_LENGTH,
    build_description_index,
    extract_description_from_overview,
    get_integration_meta_description,
    normalize_description,
    validate_description,
)
from _common import make_validator
from gen_docs_integrations import (
    build_readme_from_integration,
    create_overview,
    read_integrations_js,
)


MODE_BY_TYPE = {
    "agent_notification": "agent-notification",
    "authentication": "authentication",
    "cloud_notification": "cloud-notification",
    "collector": "collector",
    "device": "device",
    "exporter": "exporter",
    "flows": "flows",
    "logs": "logs",
    "secretstore": "secretstore",
    "service_discovery": "service_discovery",
}

MARKDOWN_SPECIAL_CHARACTERS = "*_[]<>#`~"
yaml = YAML(typ="safe")

# This covers every CommonMark block form that can exist on one line. Setext
# headings require a newline, while blank lines cannot satisfy the length and
# trimming contract.
COMMONMARK_ADVERSARIAL_DESCRIPTIONS = {
    "intraword asterisk emphasis": (
        "Monitor service a*critical latency*event behavior across reliable production systems safely.",
        r"<em\b",
    ),
    "Unicode-adjacent asterisk emphasis": (
        "Monitor service é*critical latency*é behavior across reliable production systems safely.",
        r"<em\b",
    ),
    "strong emphasis": (
        "Monitor **critical latency** behavior across reliable production systems and services safely.",
        r"<strong\b",
    ),
    "inline code": (
        "Monitor `critical latency` behavior across reliable production systems and services safely.",
        r"<code\b",
    ),
    "inline link": (
        "Monitor [critical latency](latency) behavior across reliable production systems safely.",
        r"<a\b",
    ),
    "unordered list with hyphen": (
        "- Monitor service latency and availability across production systems safely.",
        r"<ul\b",
    ),
    "unordered list with plus": (
        "+ Monitor service latency and availability across production systems safely.",
        r"<ul\b",
    ),
    "unordered list with asterisk": (
        "* Monitor service latency and availability across production systems safely.",
        r"<ul\b",
    ),
    "blockquote": (
        "> Monitor service latency and availability across production systems safely.",
        r"<blockquote\b",
    ),
    "ATX heading": (
        "# Monitor service latency and availability across production systems safely.",
        r"<h1\b",
    ),
    "indented code": (
        "    Monitor service latency and availability across production systems safely.",
        r"<pre\b",
    ),
    "fenced code": (
        "``` Monitor service latency and availability across production systems safely.",
        r"<pre\b",
    ),
    "HTML block": (
        "<div>Monitor service latency and availability across production systems safely.</div>",
        r"<div\b",
    ),
    "link reference definition": (
        "[latency]: /metrics 'Monitor service latency and availability across production systems safely.'",
        r"^$",
    ),
    "compact thematic break": (
        "-" * MIN_DESCRIPTION_LENGTH,
        r"<hr\b",
    ),
    "spaced thematic break": (
        "- " * 24 + "--",
        r"<hr\b",
    ),
}
COMMONMARK_ADVERSARIAL_DESCRIPTIONS.update(
    {
        f"ordered list with {width} digits and {delimiter!r}": (
            f"{'1' * width}{delimiter} Monitor service latency and availability across production systems safely.",
            r"<ol\b",
        )
        for width in range(1, 10)
        for delimiter in (".", ")")
    }
)

COMMONMARK_PLAIN_TEXT_DESCRIPTIONS = {
    "ordinary internal hyphens, pluses, and digits": (
        "Monitor end-to-end C++ service health for 3 production tiers with reliable metrics."
    ),
    "leading hyphen without list space": (
        "-Monitor service latency and availability across production systems safely."
    ),
    "leading plus without list space": (
        "+Monitor service latency and availability across production systems safely."
    ),
    "ten-digit non-list prefix": (
        "1234567890. Monitor service latency and availability across production systems safely."
    ),
    "hyphen prose that is not a thematic break": (
        "--- Monitor service latency and availability across production systems safely."
    ),
}

SCHEMA_REF_BY_TYPE = {
    "agent_notification": "./agent_notification.json#",
    "authentication": "./authentication.json#",
    "cloud_notification": "./cloud_notification.json#",
    "collector": "./collector.json#",
    "device": "./device.json#",
    "exporter": "./exporter.json#",
    "flows": "./flows.json#",
    "logs": "./logs.json#",
    "secretstore": "./secretstore.json#",
    "service_discovery": "./service_discovery.json#",
}

VALID_EXPLICIT_DESCRIPTIONS = {
    "exactly 50 characters": "x" * MIN_DESCRIPTION_LENGTH,
    "exactly 160 characters": "x" * MAX_DESCRIPTION_LENGTH,
    "parser-safe Unicode": "Monitor service behavior and operational health with Netdata’s real-time metrics.",
    "Unicode prose": "Monitor Κατάσταση, 数据, and café service health across reliable production systems.",
    "Unicode joiners": "Monitor Persian می‌شود text and 👩‍💻 operator workflows across reliable production systems.",
    "non-ASCII URL lookalike": "Monitor K://example identifiers as plain Unicode text across reliable production systems.",
}
VALID_EXPLICIT_DESCRIPTIONS.update(COMMONMARK_PLAIN_TEXT_DESCRIPTIONS)

INVALID_EXPLICIT_DESCRIPTIONS = {
    "non-string": 42,
    "empty": "",
    "49 characters": "x" * (MIN_DESCRIPTION_LENGTH - 1),
    "161 characters": "x" * (MAX_DESCRIPTION_LENGTH + 1),
    "50 spaces": " " * MIN_DESCRIPTION_LENGTH,
    "one character plus 49 spaces": "x" + " " * (MIN_DESCRIPTION_LENGTH - 1),
    "leading space": " " + "x" * MIN_DESCRIPTION_LENGTH,
    "trailing space": "x" * MIN_DESCRIPTION_LENGTH + " ",
    "leading tab": "\t" + "x" * MIN_DESCRIPTION_LENGTH,
    "trailing tab": "x" * MIN_DESCRIPTION_LENGTH + "\t",
    "internal tab": "x" * 25 + "\t" + "x" * 25,
    "spaces around 160 characters": " " + "x" * MAX_DESCRIPTION_LENGTH + " ",
    "Markdown link": "Monitor [PostgreSQL](https://postgresql.org) queries and connections across the server.",
    "Markdown emphasis": "Monitor **PostgreSQL** queries and connections across every production database server.",
    "Markdown single emphasis": "Monitor *PostgreSQL* queries and connections across every production database server.",
    "Markdown code": "Monitor `PostgreSQL` queries and connections across every production database server.",
    "HTML": "Monitor <strong>PostgreSQL</strong> queries and connections across every production database server.",
    "URL": "Monitor PostgreSQL queries at https://example.com/metrics across every database server.",
    "uppercase URL": "Monitor PostgreSQL queries at HTTPS://example.com/metrics across every database server.",
    "other URL scheme": "Monitor PostgreSQL queries at ftp://example.com/metrics across every database server.",
    "double quote": 'Monitor PostgreSQL queries and the "ready" state across every production database server.',
    "backslash": r"Monitor PostgreSQL queries under C:\metrics across every production database server.",
    "Unicode emphasis boundary": "Monitor service behavior around é*word*é boundaries with reliable production health metrics.",
    "Unicode line separator": "Monitor service behavior and health across production systems.\u2028Track reliable operations.",
    "Unicode paragraph separator": "Monitor service behavior and health across production systems.\u2029Track reliable operations.",
    "high surrogate": "Monitor service behavior and health across production \ud800 systems reliably.",
    "low surrogate": "Monitor service behavior and health across production \udfff systems reliably.",
}
INVALID_EXPLICIT_DESCRIPTIONS.update(
    {
        f"control U+{codepoint:04X}": "x" * 25 + chr(codepoint) + "x" * 25
        for codepoint in (*range(0x20), *range(0x7F, 0xA0))
    }
)
INVALID_EXPLICIT_DESCRIPTIONS.update(
    {label: value for label, (value, _) in COMMONMARK_ADVERSARIAL_DESCRIPTIONS.items()}
)
INVALID_EXPLICIT_DESCRIPTIONS.update(
    {
        f"Markdown special {character!r}": (
            f"Monitor service health with literal {character} metadata across reliable production systems safely."
        )
        for character in MARKDOWN_SPECIAL_CHARACTERS
    }
)

MAP_DESCRIPTION_TARGETS = {
    "docs/NIDL-Framework.md",
    "docs/developer-and-contributor-corner/dyncfg.md",
    "docs/learn.netdata.cloud/ecr-mirror.md",
    "docs/learn.netdata.cloud/installation.md",
    "docs/netdata-agent/configuration/dynamic-configuration.md",
    "docs/netdata-enterprise-evaluation.md",
    "docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md",
    "docs/observability-centralization-points/metrics-centralization-points/configuration.md",
    "docs/observability-centralization-points/metrics-centralization-points/faq.md",
    "docs/observability-centralization-points/metrics-centralization-points/replication-of-past-samples.md",
    "docs/realtime-monitoring.md",
    "docs/scalability.md",
    "docs/security-and-privacy-design/README.md",
    "docs/security-and-privacy-design/netdata-agent-security.md",
    "docs/security-and-privacy-design/netdata-cloud-security.md",
    "docs/troubleshooting/troubleshoot.md",
    "packaging/installer/methods/aws.md",
    "packaging/installer/methods/azure.md",
    "packaging/installer/methods/gcp.md",
    "src/registry/CONFIGURATION.md",
    "src/registry/README.md",
    "src/streaming/README.md",
    "src/web/api/formatters/csv/README.md",
    "src/web/api/formatters/json/README.md",
}


def remove_fenced_code(markdown):
    return re.sub(r"```.*?```|~~~.*?~~~", " ", markdown, flags=re.DOTALL)


def top_level_heading_count(markdown):
    return len(re.findall(r"^# (?!#)", remove_fenced_code(markdown), flags=re.MULTILINE))


def parse_description_with_learn_legacy_parser(description_line):
    """Apply the description-relevant behavior of Learn ingest's read_metadata()."""
    value = description_line.split(": ", 1)[1]
    try:
        if isinstance(ast.literal_eval(value), dict):
            value = ast.literal_eval(value)
    except (SyntaxError, ValueError):
        pass
    return value.strip('"')


class DescriptionNormalizationTest(unittest.TestCase):
    def test_markdown_is_reduced_to_plain_text(self):
        source = "Monitor [PostgreSQL](https://postgresql.org) `queries` and **connections** across the server."
        self.assertEqual(
            normalize_description(source, summarize=False),
            "Monitor PostgreSQL queries and connections across the server.",
        )

    def test_overview_skips_admonitions_and_uses_enough_sentences(self):
        overview = """# Example

## Overview

:::info
Setup note that must not become search metadata.
:::

Monitor short metrics. Collect request rates and latency for every service instance.
"""
        self.assertEqual(
            extract_description_from_overview(overview, for_meta=True),
            "Monitor short metrics. Collect request rates and latency for every service instance.",
        )

    def test_catalog_extraction_retains_legacy_first_sentence_behavior(self):
        overview = """## Overview

Monitor **short** metrics. Collect enough detail to pass meta-description validation.
"""
        self.assertEqual(extract_description_from_overview(overview), "Monitor **short** metrics.")

    def test_long_description_is_trimmed_at_a_word_boundary(self):
        description = normalize_description("word " * 80, summarize=False)
        self.assertLessEqual(len(description), MAX_DESCRIPTION_LENGTH)
        self.assertTrue(description.endswith("…"))
        self.assertNotIn(" …", description)

    def test_wrapping_quotes_are_removed_for_learn_frontmatter(self):
        source = '"Monitor enterprise server sensors, event logs, and hardware health across the infrastructure."'
        self.assertEqual(
            normalize_description(source, summarize=False),
            "Monitor enterprise server sensors, event logs, and hardware health across the infrastructure.",
        )

    def test_explicit_description_overrides_overview(self):
        integration = {
            "id": "test",
            "meta": {"description": "Use the explicit integration description because the overview is deliberately generic."},
            "overview": "# Test\n\n## Overview\n\nThis generic overview is long enough to be selected without the override.",
        }
        self.assertEqual(
            get_integration_meta_description(integration),
            integration["meta"]["description"],
        )

    def test_explicit_description_is_emitted_without_rewriting(self):
        description = "Monitor explicit descriptions without rewriting the author's valid plain text."
        integration = {
            "id": "test",
            "meta": {"description": description},
        }
        self.assertEqual(get_integration_meta_description(integration), description)

    def test_explicit_description_rejects_authored_violations_before_normalization(self):
        categories = [{"id": "logs", "name": "logs", "children": []}]
        for label, value in INVALID_EXPLICIT_DESCRIPTIONS.items():
            integration = {
                "id": f"invalid-{label}",
                "integration_type": "logs",
                "edit_link": "https://github.com/netdata/netdata/blob/master/integrations/logs/metadata.yaml",
                "meta": {
                    "name": "Invalid test",
                    "description": value,
                    "categories": ["logs"],
                    "icon_filename": "test.svg",
                },
                "overview": "# Invalid test\n\n## Overview\n\nMonitor a valid fallback that must not hide an invalid override.",
            }
            with self.subTest(label=label):
                with self.assertRaises(RuntimeError) as caught:
                    build_readme_from_integration(integration, categories, mode="logs")
                self.assertIsInstance(caught.exception.__cause__, ValueError)

    def test_invalid_description_fails(self):
        with self.assertRaisesRegex(ValueError, "contains a URL"):
            validate_description(
                "Monitor this service and read all metrics from https://example.com/metrics.",
                "test",
            )

        with self.assertRaisesRegex(ValueError, "frontmatter parser"):
            validate_description(
                'Monitor the service and label the special "ready" state for operators.',
                "test",
            )

    def test_duplicate_descriptions_fail(self):
        description = "Monitor distinct test services with enough accurate text for validation."
        integrations = [
            {"id": "one", "integration_type": "logs", "meta": {"description": description}},
            {"id": "two", "integration_type": "logs", "meta": {"description": description}},
        ]
        with self.assertRaisesRegex(ValueError, "Duplicate generated descriptions"):
            build_description_index(integrations)

    def test_duplicate_identity_uses_nfc_and_casefold_without_rewriting_output(self):
        composed = "Monitor Café service behavior and health across reliable production systems."
        decomposed = unicodedata.normalize("NFD", composed.upper())
        integrations = [
            {"id": "one", "integration_type": "logs", "meta": {"description": composed}},
            {"id": "two", "integration_type": "logs", "meta": {"description": decomposed}},
        ]

        with self.assertRaisesRegex(ValueError, "Duplicate generated descriptions"):
            build_description_index(integrations)

        only_decomposed = build_description_index(integrations[1:])
        self.assertEqual(only_decomposed["two"], decomposed)
        self.assertNotEqual(only_decomposed["two"], unicodedata.normalize("NFC", decomposed))


class GeneratedDocumentationDescriptionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.categories, cls.integrations = read_integrations_js("integrations/integrations.js")
        cls.documented = [
            integration
            for integration in cls.integrations
            if integration.get("integration_type") in DOCUMENTATION_TYPES
        ]

    def test_every_documentation_mode_emits_one_valid_description(self):
        seen_modes = set()
        descriptions = []

        for integration in self.documented:
            integration_type = integration["integration_type"]
            seen_modes.add(integration_type)
            _, _, _, markdown, _ = build_readme_from_integration(
                integration,
                self.categories,
                mode=MODE_BY_TYPE[integration_type],
            )
            matches = re.findall(r"^description: (.+)$", markdown, flags=re.MULTILINE)
            self.assertEqual(len(matches), 1, integration["id"])

            description = json.loads(matches[0])
            validate_description(description, integration["id"])
            self.assertEqual(description, get_integration_meta_description(integration))
            descriptions.append(unicodedata.normalize("NFC", description.casefold()))

        self.assertEqual(seen_modes, DOCUMENTATION_TYPES)
        self.assertEqual(len(descriptions), len(set(descriptions)))
        self.assertTrue(all(MIN_DESCRIPTION_LENGTH <= len(value) <= MAX_DESCRIPTION_LENGTH for value in descriptions))

    def test_logo_and_maintenance_badges_have_alt_text(self):
        integration = next(item for item in self.documented if item["integration_type"] == "collector")
        overview = create_overview(
            integration,
            integration["meta"]["monitored_instance"]["icon_filename"],
        )
        expected_name = integration["meta"]["monitored_instance"]["name"]
        self.assertIn(f'alt="{expected_name}"', overview)

        cloud_notification = next(
            item for item in self.documented if item["integration_type"] == "cloud_notification"
        )
        logo_only = create_overview(
            cloud_notification,
            cloud_notification["meta"]["icon_filename"],
            "",
        )
        self.assertIn(f'alt="{cloud_notification["meta"]["name"]}"', logo_only)

        _, _, _, _, netdata_badge = build_readme_from_integration(
            integration, self.categories, mode="collector"
        )
        self.assertIn('alt="Maintained by Netdata"', netdata_badge)

        community_integration = copy.deepcopy(integration)
        community_integration["meta"]["community"] = True
        _, _, _, _, community_badge = build_readme_from_integration(
            community_integration, self.categories, mode="collector"
        )
        self.assertIn('alt="Maintained by Community"', community_badge)


class DescriptionSchemaTest(unittest.TestCase):
    def setUp(self):
        self.cases = {
            "shared": (
                make_validator("./shared.json#/$defs/instance"),
                {
                    "name": "Test",
                    "link": "https://example.com",
                    "categories": ["data-collection.test"],
                    "icon_filename": "test.svg",
                },
            ),
            "secretstore": (
                make_validator("./secretstore.json#/$defs/meta"),
                {
                    "kind": "test",
                    "name": "Test",
                    "link": "https://example.com",
                    "icon_filename": "test.svg",
                },
            ),
            "service_discovery": (
                make_validator("./service_discovery.json#/$defs/meta"),
                {
                    "kind": "test",
                    "name": "Test",
                    "tagline": "Test targets.",
                    "link": "https://example.com",
                    "icon_filename": "test.svg",
                },
            ),
        }

    def test_all_description_schemas_accept_valid_plain_text(self):
        description = "Monitor valid service behavior with enough specific plain text for generated metadata."
        for name, (validator, base) in self.cases.items():
            with self.subTest(schema=name):
                validator.validate({**base, "description": description})

    def test_all_description_schemas_reject_invalid_author_input(self):
        for schema_name, (validator, base) in self.cases.items():
            for label, value in INVALID_EXPLICIT_DESCRIPTIONS.items():
                with self.subTest(schema=schema_name, violation=label), self.assertRaises(ValidationError):
                    validator.validate({**base, "description": value})


class DescriptionSchemaGeneratorEquivalenceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.categories, integrations = read_integrations_js("integrations/integrations.js")
        cls.integrations = {
            integration_type: copy.deepcopy(
                next(item for item in integrations if item.get("integration_type") == integration_type)
            )
            for integration_type in MODE_BY_TYPE
        }
        cls.schema_fixtures = {
            integration_type: cls._load_schema_fixture(integration)
            for integration_type, integration in cls.integrations.items()
        }

    @staticmethod
    def _description_owner(integration):
        meta = integration["meta"]
        monitored_instance = meta.get("monitored_instance")
        return monitored_instance if isinstance(monitored_instance, dict) else meta

    @classmethod
    def _load_schema_fixture(cls, integration):
        source_path = integration["edit_link"].split("/blob/master/", 1)[1]
        source = yaml.load((REPO_ROOT / source_path).read_text(encoding="utf-8"))
        expected_name = cls._description_owner(integration)["name"]

        if isinstance(source, dict) and "modules" in source:
            candidates = source["modules"]
        elif isinstance(source, list):
            candidates = source
        else:
            candidates = [source]

        matching = [
            candidate
            for candidate in candidates
            if cls._description_owner(candidate).get("name") == expected_name
        ]
        if len(matching) != 1:
            raise AssertionError(
                f"Expected one source record for {integration['integration_type']} {expected_name!r}, got {len(matching)}"
            )

        if isinstance(source, dict) and "modules" in source:
            source = {**source, "modules": [matching[0]]}
        elif isinstance(source, list):
            source = [matching[0]]
        else:
            source = matching[0]
        return source

    @classmethod
    def _with_description(cls, value, integration, schema_fixture):
        rendered = copy.deepcopy(integration)
        cls._description_owner(rendered)["description"] = value

        source = copy.deepcopy(schema_fixture)
        if isinstance(source, dict) and "modules" in source:
            source_entry = source["modules"][0]
        elif isinstance(source, list):
            source_entry = source[0]
        else:
            source_entry = source
        cls._description_owner(source_entry)["description"] = value
        return rendered, source

    def _assert_contract(self, value, expected_valid, label):
        for integration_type, integration in self.integrations.items():
            rendered, source = self._with_description(
                value,
                integration,
                self.schema_fixtures[integration_type],
            )
            validator = make_validator(SCHEMA_REF_BY_TYPE[integration_type])

            try:
                validator.validate(source)
                schema_valid = True
            except ValidationError:
                schema_valid = False

            try:
                _, _, _, markdown, _ = build_readme_from_integration(
                    rendered,
                    self.categories,
                    mode=MODE_BY_TYPE[integration_type],
                )
                generator_valid = True
            except RuntimeError:
                markdown = ""
                generator_valid = False

            with self.subTest(mode=integration_type, value=label):
                self.assertEqual(schema_valid, expected_valid)
                self.assertEqual(generator_valid, expected_valid)
                self.assertEqual(schema_valid, generator_valid)

                if expected_valid:
                    description_line = next(
                        line for line in markdown.splitlines() if line.startswith("description: ")
                    )
                    serialized = description_line.split(": ", 1)[1]
                    self.assertEqual(json.loads(serialized), value)
                    self.assertEqual(parse_description_with_learn_legacy_parser(description_line), value)

    def test_all_ten_schema_and_generator_paths_accept_the_same_boundary_values(self):
        for label, value in VALID_EXPLICIT_DESCRIPTIONS.items():
            self._assert_contract(value, True, label)

    def test_all_ten_schema_and_generator_paths_reject_the_same_adversarial_values(self):
        for label, value in INVALID_EXPLICIT_DESCRIPTIONS.items():
            self._assert_contract(value, False, label)

    def test_commonmark_adversarial_values_render_as_markup(self):
        render = MarkdownIt("commonmark").render

        for label, (value, expected_markup) in COMMONMARK_ADVERSARIAL_DESCRIPTIONS.items():
            with self.subTest(value=label):
                rendered = render(value)
                self.assertRegex(rendered, expected_markup)

    def test_commonmark_plain_text_lookalikes_remain_paragraphs(self):
        render = MarkdownIt("commonmark").render

        for label, value in COMMONMARK_PLAIN_TEXT_DESCRIPTIONS.items():
            with self.subTest(value=label):
                rendered = render(value)
                self.assertRegex(rendered, r"^<p>")
                self.assertNotRegex(rendered, r"<(?:blockquote|h[1-6]|hr|ol|pre|ul)\b")

    def test_seeded_unicode_regex_properties_match_shared_schema(self):
        validator = make_validator("./shared.json#/$defs/instance")
        base = {
            "name": "Test",
            "link": "https://example.com",
            "categories": ["data-collection.test"],
            "icon_filename": "test.svg",
        }
        rng = random.Random(0x4E455444415441)
        ascii_word = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"
        non_ascii = "éΩЖ中Kİıſ"
        punctuation = ".,:;!?-+"

        def accepts(value):
            try:
                validate_description(value, "fuzz")
                python_valid = True
            except ValueError:
                python_valid = False
            try:
                validator.validate({**base, "description": value})
                schema_valid = True
            except ValidationError:
                schema_valid = False
            self.assertEqual(schema_valid, python_valid, value)
            return python_valid

        for iteration in range(256):
            scheme_start = rng.choice(ascii_word.replace("0123456789_", "") + non_ascii)
            scheme_tail = "".join(rng.choice(ascii_word + "+.-" + non_ascii) for _ in range(4))
            value = (
                f"Monitor {scheme_start}{scheme_tail}://example identifiers as plain text across reliable production systems."
            )
            scheme = scheme_start + scheme_tail
            scheme_allowed = ascii_word.replace("_", "") + "+.-"
            has_ascii_scheme_suffix = any(
                char in ascii_word[:52]
                and all(tail_char in scheme_allowed for tail_char in scheme[index:])
                for index, char in enumerate(scheme)
            )
            expected = not has_ascii_scheme_suffix and "_" not in scheme
            self.assertEqual(accepts(value), expected, value)
            if iteration < 16:
                self._assert_contract(value, expected, f"seeded URL syntax {iteration}")

            boundary_chars = ascii_word + non_ascii + punctuation
            left = rng.choice(boundary_chars)
            right = rng.choice(boundary_chars)
            marker = rng.choice(MARKDOWN_SPECIAL_CHARACTERS)
            value = (
                f"Monitor service behavior around {left}{marker}{right} metadata with reliable production health metrics."
            )
            self.assertFalse(accepts(value), value)
            if iteration < 16:
                self._assert_contract(value, False, f"seeded Markdown special character {iteration}")

            separator = rng.choice(("\u2028", "\u2029"))
            value = f"Monitor service behavior across production systems.{separator}Track reliable health and performance."
            self.assertFalse(accepts(value), repr(value))
            if iteration < 16:
                self._assert_contract(value, False, f"seeded Unicode separator {iteration}")


class GeneratorInputFailureTest(unittest.TestCase):
    def run_generator(self, cwd, *args):
        return subprocess.run(
            [sys.executable, str(INTEGRATIONS_DIR / "gen_docs_integrations.py"), *args],
            cwd=cwd,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_check_and_generation_fail_when_generated_input_is_missing(self):
        with tempfile.TemporaryDirectory() as directory:
            for args in ((), ("--check",)):
                with self.subTest(args=args):
                    result = self.run_generator(directory, *args)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("Missing generated integrations input", result.stderr)

    def test_check_and_generation_fail_when_generated_input_is_empty_or_malformed(self):
        fixtures = {
            "empty": "",
            "empty categories": "export const categories = []export const integrations = [{}]",
            "empty integrations": "export const categories = [{}]export const integrations = []",
            "malformed": "export const categories = [export const integrations = []",
        }
        for label, content in fixtures.items():
            with tempfile.TemporaryDirectory() as directory:
                integrations_dir = Path(directory) / "integrations"
                integrations_dir.mkdir()
                (integrations_dir / "integrations.js").write_text(content, encoding="utf-8")
                for args in ((), ("--check",)):
                    with self.subTest(label=label, args=args):
                        result = self.run_generator(directory, *args)
                        self.assertNotEqual(result.returncode, 0)

    def test_unknown_collector_fails(self):
        for args in (("--collector", "missing.plugin/missing"), ("--check", "--collector", "missing.plugin/missing")):
            with self.subTest(args=args):
                result = self.run_generator(REPO_ROOT, *args)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("no matching collector found", result.stderr)


class DocumentationSourceRegressionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.categories, cls.integrations = read_integrations_js("integrations/integrations.js")

    def test_map_descriptions_are_present_valid_and_unique(self):
        map_data = yaml.load((REPO_ROOT / "docs/.map/map.yaml").read_text(encoding="utf-8"))
        descriptions = []
        targeted = set()

        def walk(nodes):
            for node in nodes or []:
                if not isinstance(node, dict):
                    continue
                meta = node.get("meta", {})
                description = meta.get("description")
                edit_url = meta.get("edit_url", "")
                if description:
                    descriptions.append(description)
                    for target in MAP_DESCRIPTION_TARGETS:
                        if edit_url.endswith(target):
                            validate_description(description, edit_url)
                            targeted.add(target)
                walk(node.get("items"))

        walk(map_data["sidebar"])
        self.assertEqual(targeted, MAP_DESCRIPTION_TARGETS)
        identities = {unicodedata.normalize("NFC", value.casefold()) for value in descriptions}
        self.assertEqual(len(descriptions), len(identities))

    def test_affected_pages_have_one_top_level_heading(self):
        source_pages = {
            "src/ml/ml-configuration.md",
            "src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md",
            "src/web/api/queries/stddev/README.md",
        }
        for relative_path in source_pages:
            markdown = (REPO_ROOT / relative_path).read_text(encoding="utf-8")
            self.assertEqual(top_level_heading_count(markdown), 1, relative_path)

        sms = next(integration for integration in self.integrations if integration.get("id") == "notify-sms")
        _, _, _, markdown, _ = build_readme_from_integration(
            sms,
            self.categories,
            mode="agent-notification",
        )
        self.assertEqual(top_level_heading_count(markdown), 1, "notify-sms")


if __name__ == "__main__":
    unittest.main()
