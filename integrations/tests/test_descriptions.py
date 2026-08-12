#!/usr/bin/env python3

import copy
import json
import re
import sys
import unittest
from pathlib import Path

import yaml

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

THIN_SOURCE_DOCS = {
    "docs/Demo-Sites.md",
    "docs/category-overview-pages/maintenance-operations-on-netdata-agents.md",
    "docs/dashboards-and-charts/themes.md",
    "docs/developer-and-contributor-corner/README.md",
    "docs/developer-and-contributor-corner/build-the-netdata-agent-yourself.md",
    "docs/netdata-ai/troubleshooting/index.md",
    "docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md",
    "src/web/api/exporters/shell/README.md",
    "src/web/api/formatters/value/README.md",
    "src/web/api/queries/max/README.md",
    "src/web/api/queries/min/README.md",
    "tests/health_mgmtapi/README.md",
}

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


def visible_word_count(markdown):
    text = remove_fenced_code(markdown)
    text = re.sub(r"!\[[^]]*\]\([^)]*\)", " ", text)
    text = re.sub(r"\[([^]]+)\]\([^)]*\)", r"\1", text)
    text = re.sub(r"<[^>]+>", " ", text)
    return len(re.findall(r"\b[\w'-]+\b", text))


def top_level_heading_count(markdown):
    return len(re.findall(r"^# (?!#)", remove_fenced_code(markdown), flags=re.MULTILINE))


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
            descriptions.append(description.casefold())

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


class DocumentationSourceRegressionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.categories, cls.integrations = read_integrations_js("integrations/integrations.js")

    def test_map_descriptions_are_present_valid_and_unique(self):
        map_data = yaml.safe_load((REPO_ROOT / "docs/.map/map.yaml").read_text(encoding="utf-8"))
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
        self.assertEqual(len(descriptions), len({value.casefold() for value in descriptions}))

    def test_source_owned_thin_pages_exceed_300_visible_words(self):
        for relative_path in sorted(THIN_SOURCE_DOCS):
            markdown = (REPO_ROOT / relative_path).read_text(encoding="utf-8")
            self.assertGreater(visible_word_count(markdown), 300, relative_path)

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
