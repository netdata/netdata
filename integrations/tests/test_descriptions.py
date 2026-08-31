#!/usr/bin/env python3

import ast
import copy
import importlib.util
import json
import os
import random
import re
# Tests execute only a fixed repository-local generator.
import subprocess  # nosec B404
import sys
import tempfile
import unicodedata
import unittest
from collections import Counter
from pathlib import Path

from jsonschema import ValidationError
from markdown_it import MarkdownIt
from ruamel.yaml import YAML

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = INTEGRATIONS_DIR.parent
sys.path.insert(0, str(INTEGRATIONS_DIR))

from descriptions import (  # noqa: E402
    DOCUMENTATION_TYPES,
    MAX_DESCRIPTION_LENGTH,
    MIN_DESCRIPTION_LENGTH,
    build_description_index,
    description_report,
    extract_description_from_overview,
    get_description_override,
    get_integration_meta_description,
    normalize_description,
    parentheses_are_balanced,
    validate_description,
)
from _common import make_validator  # noqa: E402
from gen_docs_integrations import (  # noqa: E402
    _select_integrations,
    _validate_complete_description_corpus,
    build_readme_from_integration,
    create_overview,
    read_integrations_js,
)
from gen_doc_collector_page import get_integration_description  # noqa: E402
from gen_npm_catalog import (  # noqa: E402
    PAGE_DESCRIPTIONS,
    build_device_modules,
    is_device_catalog_profile,
    load_profiles,
    make_entry,
    profile_display_name,
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


class NPMCatalogProfileVisibilityTest(unittest.TestCase):
    def test_internal_topology_roles_are_not_device_integrations(self):
        self.assertFalse(is_device_catalog_profile("_std-topology-ip-mib.yaml"))
        self.assertFalse(is_device_catalog_profile("topology-role-qbridge.yaml"))
        self.assertTrue(is_device_catalog_profile("cisco-catalyst.yaml"))

        modules = build_device_modules(load_profiles())
        self.assertTrue(modules)
        method_descriptions = [
            module["overview"]["data_collection"]["method_description"]
            for module in modules
        ]
        self.assertFalse(any("topology-role-" in value for value in method_descriptions))

    def test_mikrotik_profile_names_preserve_product_capitalization(self):
        self.assertEqual(profile_display_name("mikrotik-router.yaml", "MikroTik", "Router"), "MikroTik Router")
        self.assertEqual(profile_display_name("mikrotik-swos.yaml", "MikroTik", "Switch"), "MikroTik Switch")

    def test_snmp_support_collection_is_not_attached_to_unrelated_catalog_entries(self):
        common = {
            "name": "Test",
            "link": "",
            "categories": ["data-collection.networking"],
            "icon": "netdata.png",
            "keywords": ["test"],
            "ov": {},
        }
        snmp = make_entry(**common)
        topology = make_entry(**common, module_name="snmp_topology")
        traps = make_entry(**common, module_name="snmp_traps")
        non_snmp = make_entry(**common, plugin_name="netdata", module_name="streaming")

        self.assertTrue(snmp["troubleshooting"]["problems"]["list"])
        self.assertTrue(topology["troubleshooting"]["problems"]["list"])
        self.assertFalse(traps["troubleshooting"]["problems"]["list"])
        self.assertFalse(non_snmp["troubleshooting"]["problems"]["list"])


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
    "leading plus without list space": (
        "+Monitor service latency and availability across production systems safely."
    ),
    "ten-digit non-list prefix": (
        "1234567890. Monitor service latency and availability across production systems safely."
    ),
    "Arabic-Indic digits are not a CommonMark list": (
        "١٢٣٤٥٦٧٨٩. Monitor service latency and availability across production systems safely."
    ),
    "Devanagari digits are not a CommonMark list": (
        "१२३४५६७८९. Monitor service latency and availability across production systems safely."
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
    "non-ASCII URL lookalike": (
        "Monitor K://example identifiers as plain Unicode text across reliable production systems."
    ),
    "balanced nested parentheses": (
        "Monitor service health (including nested (optional) detail) across reliable production systems."
    ),
    "internal colon": (
        "Monitor service health by state: ready, degraded, and failed across reliable production systems."
    ),
    "non-terminal ellipsis": (
        "Monitor service health across ready… degraded, and failed states with reliable production metrics."
    ),
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
    "leading hyphen without list space": (
        "-Monitor service latency and availability across production systems safely."
    ),
    "hyphen prose that is not a thematic break": (
        "--- Monitor service latency and availability across production systems safely."
    ),
    "Unicode emphasis boundary": (
        "Monitor service behavior around é*word*é boundaries with reliable production health metrics."
    ),
    "Unicode line separator": (
        "Monitor service behavior and health across production systems.\u2028Track reliable operations."
    ),
    "Unicode paragraph separator": (
        "Monitor service behavior and health across production systems.\u2029Track reliable operations."
    ),
    "high surrogate": "Monitor service behavior and health across production \ud800 systems reliably.",
    "low surrogate": "Monitor service behavior and health across production \udfff systems reliably.",
    "terminal colon": "Monitor service health and behavior across reliable production systems safely:",
    "terminal ellipsis": "Monitor service health and behavior across reliable production systems safely…",
    "terminal ASCII ellipsis": "Monitor service health and behavior across reliable production systems safely...",
    "missing closing parenthesis": (
        "Monitor service health (including optional detail across reliable production systems safely."
    ),
    "unexpected closing parenthesis": (
        "Monitor service health including optional detail) across reliable production systems safely."
    ),
    "misordered parentheses": (
        "Monitor service health )(with optional detail across reliable production systems safely."
    ),
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
    (
        "docs/observability-centralization-points/metrics-centralization-points/"
        "clustering-and-high-availability-of-netdata-parents.md"
    ),
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
    "src/libnetdata/clocks/README.md",
    "src/libnetdata/socket/README.md",
    "src/streaming/README.md",
    "src/web/api/formatters/csv/README.md",
    "src/web/api/formatters/json/README.md",
}

LIBNETDATA_REFERENCE_LINKS = {
    "src/libnetdata/clocks/README.md": {
        "clocks.h",
        "time_t_arithmetic.h",
    },
    "src/libnetdata/socket/README.md": {
        "connect-to.h",
        "listen-sockets.h",
        "nd-poll.h",
        "nd-sock.h",
        "poll-events.h",
        "security.h",
        "socket-peers.h",
        "socket.h",
    },
}

ACCURACY_OVERRIDE_TARGETS = {
    "go.d.plugin-azure_monitor-Azure_API_Management",
    "go.d.plugin-azure_monitor-Azure_Application_Gateway",
    "go.d.plugin-azure_monitor-Azure_Application_Insights",
    "go.d.plugin-azure_monitor-Azure_Cache_for_Redis",
    "go.d.plugin-azure_monitor-Azure_Cognitive_Services",
    "go.d.plugin-azure_monitor-Azure_Container_Apps",
    "go.d.plugin-azure_monitor-Azure_Container_Instances",
    "go.d.plugin-azure_monitor-Azure_Container_Registry",
    "go.d.plugin-azure_monitor-Azure_Data_Explorer_Cluster",
    "go.d.plugin-azure_monitor-Azure_ExpressRoute_Circuit",
    "go.d.plugin-azure_monitor-Azure_ExpressRoute_Gateway",
    "go.d.plugin-azure_monitor-Azure_Kubernetes_Service_Cluster",
    "go.d.plugin-azure_monitor-Azure_Load_Balancer",
    "go.d.plugin-azure_monitor-Azure_Log_Analytics_Workspace",
    "go.d.plugin-azure_monitor-Azure_Machine_Learning_Workspace",
    "go.d.plugin-azure_monitor-Azure_MySQL_Flexible_Server",
    "go.d.plugin-azure_monitor-Azure_PostgreSQL_Flexible_Server",
    "go.d.plugin-azure_monitor-Azure_SQL_Elastic_Pool",
    "go.d.plugin-azure_monitor-Azure_SQL_Managed_Instance",
    "go.d.plugin-azure_monitor-Azure_Storage_Account",
    "go.d.plugin-azure_monitor-Azure_Stream_Analytics_Job",
    "go.d.plugin-azure_monitor-Azure_Synapse_Analytics_Workspace",
    "go.d.plugin-azure_monitor-Azure_Virtual_Machine",
    "go.d.plugin-azure_monitor-Azure_Virtual_Machine_Scale_Set",
    "go.d.plugin-ntpd-NTPd",
    "go.d.plugin-zookeeper-ZooKeeper",
    "windows.plugin-PerflibHyperV-Hyper-V",
}

TERMINAL_ELLIPSIS_OVERRIDE_TARGETS = {
    "debugfs.plugin-libsensors-Linux_Hardware_Sensors_(libsensors)",
    "export-blueflood",
    "export-graphite",
    "export-json",
    "export-kairosdb",
    "export-mongodb",
    "export-opentsdb",
    "go.d.plugin-ap-Access_Points",
    "go.d.plugin-clickhouse-ClickHouse",
    "go.d.plugin-ethtool-Optical_modules",
    "go.d.plugin-nginx-NGINX",
    "go.d.plugin-nginxunit-NGINX_Unit",
    "go.d.plugin-redis-Redis",
    "go.d.plugin-snmp-A10_Thunder",
    "go.d.plugin-snmp-APC_PDU",
    "go.d.plugin-snmp-APC_UPS",
    "go.d.plugin-snmp-Cisco_ICM",
    "go.d.plugin-snmp-Cisco_SB",
    "go.d.plugin-snmp-Cisco_UCS",
    "go.d.plugin-snmp-Eaton_Epdu",
    "go.d.plugin-snmp-Eaton_UPS",
    "go.d.plugin-snmp-Exagrid",
    "go.d.plugin-snmp-HPE_MSA",
    "go.d.plugin-snmp-HP_ILO",
    "go.d.plugin-snmp-HP_Ilo4",
    "go.d.plugin-snmp-IDRAC",
    "go.d.plugin-snmp-Isilon",
    "go.d.plugin-snmp-Peplink",
    "go.d.plugin-snmp_topology-BGP_Peering_Topology",
    "go.d.plugin-snmp_traps-SNMP_Trap_Node_Attribution",
    "go.d.plugin-vsphere-vSphere_Topology",
    "ibm.d.plugin-db2-IBM_DB2",
    "ibm.d.plugin-websphere_pmi-IBM_WebSphere_PMI",
    "idlejitter.plugin-idlejitter.plugin-Idle_OS_Jitter",
    "network-viewer.plugin-network-viewer-Live_Network_Connections",
    "proc.plugin-/sys/devices/system/edac/mc-Memory_modules_(DIMMs)",
    "proc.plugin-/sys/kernel/mm/ksm-Kernel_Same-Page_Merging",
    "scim",
    "service-discovery-http",
    "windows.plugin-PerflibAD-Active_Directory",
}

REVIEWED_DESCRIPTION_TARGETS = {
    "cgroups.plugin-/sys/fs/cgroup-Kubernetes_Containers": (
        "Monitor Kubernetes pod and container CPU, memory, disk I/O, and network utilization through Linux cgroups."
    ),
    "debugfs.plugin-libsensors-Linux_Hardware_Sensors_(libsensors)": (
        "Collect hardware sensor readings from Linux hwmon chips, including temperature, voltage, current, "
        "fan speed, power, energy, humidity, and intrusion state."
    ),
    "cgroups.plugin-/sys/fs/cgroup-Systemd_Services": (
        "Monitor systemd service CPU, memory, disk I/O, and page cache activity through Linux cgroups."
    ),
    "debugfs.plugin-audit-Linux_Audit_Subsystem": (
        "Monitor Linux kernel audit status, backlog utilization, lost events, failure mode, and enabled state."
    ),
    "ebpf.plugin-processes-eBPF_Processes": (
        "Monitor Linux process and thread creation by tracing kernel task-creation functions with eBPF."
    ),
    "ebpf.plugin-swap-eBPF_SWAP": (
        "Monitor Linux swap reads and writes by tracing kernel swap I/O functions with eBPF."
    ),
    "go.d.plugin-dnsmasq_dhcp-Dnsmasq_DHCP": (
        "Monitor Dnsmasq DHCP leases and utilization across configured address ranges."
    ),
    "go.d.plugin-k8s_kubelet-Kubelet": (
        "Monitor Kubernetes Kubelet containers, pods, runtime operations, storage, and request metrics."
    ),
    "go.d.plugin-ntpd-NTPd": (
        "Monitor a configured NTP daemon's clock offset, jitter, frequency, dispersion, stratum, precision, "
        "and optional peer timing metrics."
    ),
    "go.d.plugin-prometheus-AWS_Quota": (
        "Monitor AWS Service Quotas and usage exposed by the AWS Quota Exporter."
    ),
    "go.d.plugin-snmp_traps-SNMP_Trap_Reverse_DNS_Enrichment": (
        "Add reverse DNS hostnames to SNMP traps through cached PTR lookups of their source addresses."
    ),
    "proc.plugin-/proc/net/softnet_stat-Softnet_Statistics": (
        "Monitor Linux softnet packet processing, including drops, quota exhaustion, RPS, flow-limit, and GRO activity."
    ),
    "proc.plugin-/proc/net/sockstat-Socket_statistics": (
        "Monitor Linux socket usage across address families plus IPv4 protocol memory statistics reported "
        "through procfs."
    ),
    "proc.plugin-/sys/kernel/mm/ksm-Kernel_Same-Page_Merging": (
        "Monitor Linux Kernel Samepage Merging activity and effectiveness, including shared pages, savings, "
        "ratios, and memory deduplication behavior."
    ),
    "proc.plugin-/sys/block/zram-ZRAM": (
        "Monitor zRAM device capacity, compression, memory use, and I/O activity on Linux systems."
    ),
    "proc.plugin-/sys/class/infiniband-InfiniBand": (
        "Monitor InfiniBand network interface traffic and errors."
    ),
    "proc.plugin-/proc/spl/kstat/zfs/arcstats-ZFS_Adaptive_Replacement_Cache": (
        "Monitor ZFS Adaptive Replacement Cache (ARC) performance and memory statistics."
    ),
    "windows.plugin-PerflibNetFramework-NET_Framework": (
        "Monitor runtime and performance statistics for applications built with .NET Framework through Perflib."
    ),
    "windows.plugin-PerflibProcessor-Processor": (
        "Monitor processor performance statistics on Windows hosts through Perflib."
    ),
    "freebsd.plugin-dev.cpu.0.freq-dev.cpu.0.freq": (
        "Monitor the current FreeBSD CPU scaling frequency through sysctl."
    ),
    "freebsd.plugin-getifaddrs-getifaddrs": (
        "Monitor FreeBSD network interface traffic, packets, errors, drops, and collision events."
    ),
    "freebsd.plugin-kern.ipc.sem-kern.ipc.sem": (
        "Monitor FreeBSD System V IPC semaphore set and semaphore counts."
    ),
    "freebsd.plugin-vm.vmtotal-vm.vmtotal": (
        "Monitor FreeBSD active, running, and blocked process counts plus total real memory in use."
    ),
    "windows.plugin-PerflibNetwork-Network_Subsystem": (
        "Monitor Windows network interface traffic, errors, drops, queue length, and offload activity through Perflib."
    ),
}

NPM_PAGE_DESCRIPTION_TARGETS = {
    "A10 Thunder",
    "APC PDU",
    "APC UPS",
    "BGP Peering Topology",
    "Cisco ICM",
    "Cisco SB",
    "Cisco UCS",
    "Eaton Epdu",
    "Eaton UPS",
    "Exagrid",
    "HP ILO",
    "HP Ilo4",
    "HPE MSA",
    "IDRAC",
    "Isilon",
    "Live Network Connections",
    "Peplink",
    "SNMP Trap Reverse DNS Enrichment",
    "SNMP Trap Node Attribution",
    "vSphere Topology",
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


def load_learn_read_metadata():
    """Load Learn's real parser when the cross-repository CI checkout is available."""
    ingest_value = os.environ.get("LEARN_INGEST_PATH")
    if not ingest_value:
        return None

    ingest_path = Path(ingest_value).resolve()
    if not ingest_path.is_file():
        raise AssertionError(f"LEARN_INGEST_PATH is not a file: {ingest_path}")

    spec = importlib.util.spec_from_file_location("netdata_learn_ingest_contract", ingest_path)
    if spec is None or spec.loader is None:
        raise AssertionError(f"Cannot load Learn ingest parser: {ingest_path}")

    module = importlib.util.module_from_spec(spec)
    sys.path.insert(0, str(ingest_path.parent))
    try:
        spec.loader.exec_module(module)
    finally:
        sys.path.pop(0)
    return module.read_metadata


class DescriptionNormalizationTest(unittest.TestCase):
    def test_non_mapping_meta_fails_with_an_actionable_error(self):
        for meta in (None, [], "invalid", 42, True):
            with self.subTest(meta=meta), self.assertRaisesRegex(
                ValueError,
                "meta must be a mapping",
            ):
                get_description_override({"id": "invalid-meta", "meta": meta})

    def test_catalog_fallback_rejects_an_invalid_explicit_description(self):
        integration = {
            "id": "collector-test-invalid",
            "meta": {
                "monitored_instance": {
                    "name": "Invalid Description Fixture",
                    "description": "too short",
                }
            },
            "overview": "",
        }
        with self.assertRaisesRegex(ValueError, "outside 50-160 characters"):
            get_integration_description(integration)

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

    def test_overview_skips_complete_and_indented_code_blocks(self):
        overview = """# Example

## Overview

```python
print("not prose")
```

    indented_code_is_not_prose()

Monitor service latency, request rates, and availability across reliable production systems.
"""
        self.assertEqual(
            extract_description_from_overview(overview, for_meta=True),
            "Monitor service latency, request rates, and availability across reliable production systems.",
        )

    def test_overview_rejects_an_unterminated_fence_without_prose(self):
        overview = """# Example

## Overview

```python
print("the rest of this overview is code")
"""
        self.assertIsNone(extract_description_from_overview(overview, for_meta=True))

    def test_overview_heading_must_be_a_top_level_markdown_heading(self):
        false_heading_contexts = {
            "fenced code": "```markdown\n## Overview\nFenced example text.\n```",
            "indented code": "    ## Overview\n    Indented example text.",
            "blockquote": "> ## Overview\n> Quoted example text.",
            "HTML code block": "<pre>\n## Overview\nHTML example text.\n</pre>",
        }
        expected = (
            "Monitor service latency, request rates, and availability across reliable production systems."
        )

        for label, false_heading in false_heading_contexts.items():
            overview = (
                f"# Example\n\n{false_heading}\n\n## Overview\n\n{expected}\n"
            )
            with self.subTest(context=label):
                self.assertEqual(
                    extract_description_from_overview(overview, for_meta=True),
                    expected,
                )

    def test_catalog_extraction_retains_legacy_first_sentence_behavior(self):
        overview = """## Overview

Monitor **short** metrics. Collect enough detail to pass meta-description validation.
"""
        self.assertEqual(extract_description_from_overview(overview), "Monitor **short** metrics.")

    def test_mechanical_truncation_fails_final_validation(self):
        integration = {
            "id": "terminal-ellipsis",
            "overview": "# Example\n\n## Overview\n\n" + "word " * 80,
        }
        description = extract_description_from_overview(integration["overview"], for_meta=True)
        self.assertLessEqual(len(description), MAX_DESCRIPTION_LENGTH)
        self.assertTrue(description.endswith("…"))
        self.assertNotIn(" …", description)
        with self.assertRaisesRegex(ValueError, "ends with an ellipsis"):
            get_integration_meta_description(integration)

    def test_terminal_colon_extraction_fails_final_validation(self):
        integration = {
            "id": "terminal-colon",
            "overview": "# Example\n\n## Overview\n\nMonitor service metrics covering:\n\n- latency\n- errors",
        }
        self.assertEqual(
            extract_description_from_overview(integration["overview"], for_meta=True),
            "Monitor service metrics covering:",
        )
        with self.assertRaisesRegex(ValueError, "ends with a colon"):
            get_integration_meta_description(integration)

    def test_truncation_inside_parentheses_fails_final_validation(self):
        integration = {
            "id": "unbalanced-truncation",
            "overview": (
                "# Example\n\n## Overview\n\nMonitor service health across production systems "
                "with detailed resource statistics (including CPU utilization, memory pressure, "
                "storage latency, network throughput, process state, and availability) for operators."
            ),
        }
        extracted = extract_description_from_overview(integration["overview"], for_meta=True)
        self.assertFalse(parentheses_are_balanced(extracted))
        with self.assertRaisesRegex(ValueError, "contains unbalanced parentheses"):
            get_integration_meta_description(integration)

    def test_wrapping_quotes_are_removed_for_learn_frontmatter(self):
        source = '"Monitor enterprise server sensors, event logs, and hardware health across the infrastructure."'
        self.assertEqual(
            normalize_description(source, summarize=False),
            "Monitor enterprise server sensors, event logs, and hardware health across the infrastructure.",
        )

    def test_explicit_description_overrides_overview(self):
        integration = {
            "id": "test",
            "meta": {
                "description": "Use the explicit integration description because the overview is deliberately generic."
            },
            "overview": (
                "# Test\n\n## Overview\n\n"
                "This generic overview is long enough to be selected without the override."
            ),
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
                "overview": (
                    "# Invalid test\n\n## Overview\n\n"
                    "Monitor a valid fallback that must not hide an invalid override."
                ),
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
            self.assertFalse(description.endswith(":"), integration["id"])
            self.assertFalse(description.endswith("…"), integration["id"])
            self.assertFalse(description.endswith("..."), integration["id"])
            self.assertTrue(parentheses_are_balanced(description), integration["id"])
            descriptions.append(unicodedata.normalize("NFC", description.casefold()))

        self.assertEqual(seen_modes, DOCUMENTATION_TYPES)
        self.assertEqual(len(descriptions), len(set(descriptions)))
        self.assertTrue(all(MIN_DESCRIPTION_LENGTH <= len(value) <= MAX_DESCRIPTION_LENGTH for value in descriptions))

    def test_mechanical_extraction_never_fuses_underscored_identifiers(self):
        source = "Monitor kernel audit state through `NETLINK_AUDIT` with reliable backlog and loss metrics."
        derived = normalize_description(source, summarize=True)
        self.assertIn("NETLINK_AUDIT", derived)
        with self.assertRaisesRegex(ValueError, "Markdown-special character"):
            validate_description(derived, "underscored-identifier")

    def test_complete_corpus_and_override_inventories(self):
        report = description_report(self.documented)
        self.assertEqual(report["pages"], len(self.documented))
        self.assertEqual(
            report["explicit_overrides"] + report["mechanical_descriptions"],
            report["pages"],
        )

        by_id = {integration["id"]: integration for integration in self.documented}
        self.assertTrue(ACCURACY_OVERRIDE_TARGETS.issubset(by_id))
        for integration_id in ACCURACY_OVERRIDE_TARGETS:
            self.assertIsNotNone(get_description_override(by_id[integration_id]), integration_id)

        self.assertEqual(len(TERMINAL_ELLIPSIS_OVERRIDE_TARGETS), 40)
        self.assertTrue(TERMINAL_ELLIPSIS_OVERRIDE_TARGETS.issubset(by_id))
        for integration_id in TERMINAL_ELLIPSIS_OVERRIDE_TARGETS:
            description = get_description_override(by_id[integration_id])
            self.assertIsNotNone(description, integration_id)
            self.assertFalse(description.endswith("…"), integration_id)
            self.assertFalse(description.endswith("..."), integration_id)

    def test_scoped_generation_rejects_a_collision_outside_the_selection(self):
        description = (
            "Monitor duplicate service latency and availability across reliable production systems."
        )
        integrations = [
            {
                "id": "go.d.plugin-one-One",
                "integration_type": "collector",
                "meta": {
                    "plugin_name": "go.d.plugin",
                    "module_name": "one",
                    "monitored_instance": {"description": description},
                },
            },
            {
                "id": "go.d.plugin-two-Two",
                "integration_type": "collector",
                "meta": {
                    "plugin_name": "go.d.plugin",
                    "module_name": "two",
                    "monitored_instance": {"description": description},
                },
            },
        ]

        self.assertEqual(len(_select_integrations(integrations, "go.d.plugin/one")), 1)
        with self.assertRaisesRegex(ValueError, "Duplicate generated descriptions"):
            _validate_complete_description_corpus(integrations)

    def test_reviewed_descriptions_match_authoritative_source_copy(self):
        by_id = {integration["id"]: integration for integration in self.documented}
        self.assertLessEqual(REVIEWED_DESCRIPTION_TARGETS.keys(), by_id.keys())
        for integration_id, expected in REVIEWED_DESCRIPTION_TARGETS.items():
            with self.subTest(integration_id=integration_id):
                self.assertEqual(get_integration_meta_description(by_id[integration_id]), expected)

        systemd = by_id["cgroups.plugin-/sys/fs/cgroup-Systemd_Services"]
        self.assertIn(
            "Monitor systemd service resource utilization — CPU, memory, and disk I/O — via Linux cgroups.",
            systemd["overview"],
        )
        self.assertNotIn("network", systemd["overview"].casefold())

    def test_cgroups_kubernetes_overview_uses_the_schema_key(self):
        source = yaml.load((REPO_ROOT / "src/collectors/cgroups.plugin/metadata.yaml").read_text(encoding="utf-8"))
        kubernetes = next(
            module
            for module in source["modules"]
            if module["meta"]["monitored_instance"]["name"] == "Kubernetes Containers"
        )
        self.assertIn("data_collection", kubernetes["overview"])
        self.assertNotIn("data-collection", kubernetes["overview"])

    def test_npm_page_description_mapping_is_fully_consumed(self):
        source = yaml.load(
            (REPO_ROOT / "src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml").read_text(encoding="utf-8")
        )
        described_names = Counter(
            module["meta"]["monitored_instance"]["name"]
            for module in source["modules"]
            if "description" in module["meta"]["monitored_instance"]
        )
        self.assertEqual(set(PAGE_DESCRIPTIONS), NPM_PAGE_DESCRIPTION_TARGETS)
        self.assertEqual(described_names, Counter({name: 1 for name in NPM_PAGE_DESCRIPTION_TARGETS}))

    def test_ibm_page_descriptions_are_owned_by_module_sources(self):
        modules = (
            "src/go/plugin/ibm.d/modules/db2",
            "src/go/plugin/ibm.d/modules/websphere/pmi",
        )
        for module_dir in modules:
            with self.subTest(module=module_dir):
                source = yaml.load((REPO_ROOT / module_dir / "module.yaml").read_text(encoding="utf-8"))
                generated = yaml.load((REPO_ROOT / module_dir / "metadata.yaml").read_text(encoding="utf-8"))
                description = generated["modules"][0]["meta"]["monitored_instance"]["description"]
                self.assertEqual(description, source["page_description"])

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
        cloud_notification_name = cloud_notification["meta"]["name"]
        self.assertIn(f'alt="{cloud_notification_name}"', logo_only)

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
        learn_read_metadata = load_learn_read_metadata()
        cls.learn_read_metadata = (
            staticmethod(learn_read_metadata) if learn_read_metadata is not None else None
        )
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
                f"Expected one source record for {integration['integration_type']} "
                f"{expected_name!r}, got {len(matching)}"
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
                    if self.learn_read_metadata is not None:
                        metadata = self.learn_read_metadata(f"---\n{description_line}\n---")
                        self.assertEqual(metadata["description"], value)

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
        # A fixed seed makes these non-security property fixtures reproducible.
        rng = random.Random(0x4E455444415441)  # nosec B311
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
                f"Monitor {scheme_start}{scheme_tail}://example identifiers as plain text "
                "across reliable production systems."
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
                f"Monitor service behavior around {left}{marker}{right} metadata with reliable "
                "production health metrics."
            )
            self.assertFalse(accepts(value), value)
            if iteration < 16:
                self._assert_contract(value, False, f"seeded Markdown special character {iteration}")

            separator = rng.choice(("\u2028", "\u2029"))
            value = (
                f"Monitor service behavior across production systems.{separator}"
                "Track reliable health and performance."
            )
            self.assertFalse(accepts(value), repr(value))
            if iteration < 16:
                self._assert_contract(value, False, f"seeded Unicode separator {iteration}")


class GeneratorInputFailureTest(unittest.TestCase):
    def run_generator(self, cwd, *args):
        # The command is a fixed local script plus controlled test fixtures.
        return subprocess.run(  # nosec B603
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

    def test_description_validation_failure_is_concise(self):
        description = (
            "Monitor duplicate service latency and availability across reliable production systems."
        )
        integrations = [
            {
                "id": f"go.d.plugin-{module}-{module.title()}",
                "integration_type": "collector",
                "meta": {
                    "plugin_name": "go.d.plugin",
                    "module_name": module,
                    "monitored_instance": {"description": description},
                },
            }
            for module in ("one", "two")
        ]
        with tempfile.TemporaryDirectory() as directory:
            integrations_dir = Path(directory) / "integrations"
            integrations_dir.mkdir()
            (integrations_dir / "integrations.js").write_text(
                "export const categories = "
                + json.dumps([{"id": "data-collection"}])
                + "\nexport const integrations = "
                + json.dumps(integrations),
                encoding="utf-8",
            )
            result = self.run_generator(directory, "--check", "--collector", "go.d.plugin/one")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Error: Duplicate generated descriptions", result.stderr)
        self.assertNotIn("Traceback", result.stderr)


class DocumentationSourceRegressionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.categories, cls.integrations = read_integrations_js("integrations/integrations.js")

    def test_ibm_d_guide_maps_all_module_directories_to_real_selectors(self):
        guide = (REPO_ROOT / ".agents/skills/integrations-lifecycle/ibm-d.md").read_text(encoding="utf-8")
        self.assertIn("`<module-dir>` is the path relative to", guide)
        self.assertIn("`<module-name>` is the exact `name` value", guide)
        self.assertIn(
            "python3 integrations/gen_docs_integrations.py -c ibm.d.plugin/<module-name>",
            guide,
        )
        self.assertNotIn("ibm.d.plugin/<m>", guide)
        self.assertNotIn("ibm.d/<m>", guide)

        modules = {
            "as400": "as400",
            "db2": "db2",
            "mq": "mq",
            "websphere/jmx": "websphere_jmx",
            "websphere/mp": "websphere_mp",
            "websphere/pmi": "websphere_pmi",
        }
        actual_names = {
            item["meta"]["module_name"]
            for item in self.integrations
            if item.get("meta", {}).get("plugin_name") == "ibm.d.plugin"
        }
        self.assertEqual(actual_names, set(modules.values()))

        for module_dir, module_name in modules.items():
            with self.subTest(module_dir=module_dir, module_name=module_name):
                source = yaml.load(
                    (
                        REPO_ROOT / "src/go/plugin/ibm.d/modules" / module_dir / "module.yaml"
                    ).read_text(encoding="utf-8")
                )
                self.assertEqual(source["name"], module_name)
                selected = _select_integrations(self.integrations, f"ibm.d.plugin/{module_name}")
                self.assertTrue(selected)
                self.assertEqual(
                    {item["meta"]["module_name"] for item in selected},
                    {module_name},
                )

    def test_ci_generates_authoritative_source_metadata_before_public_docs(self):
        workflows = (
            REPO_ROOT / ".github/workflows/check-markdown.yml",
            REPO_ROOT / ".github/workflows/generate-integrations.yml",
        )
        for workflow in workflows:
            with self.subTest(workflow=workflow.name):
                data = yaml.load(workflow.read_text(encoding="utf-8"))
                self.assertEqual(len(data["jobs"]), 1)
                trigger = "push" if workflow.name == "generate-integrations.yml" else "pull_request"
                self.assertIn(
                    "src/go/plugin/ibm.d/modules/**/init.go",
                    data["on"][trigger]["paths"],
                )
                steps = next(iter(data["jobs"].values()))["steps"]
                by_name = {step["name"]: step for step in steps}
                names = [step["name"] for step in steps]
                source_metadata = by_name["Generate Source Metadata"]["run"]
                self.assertIn("python3 integrations/gen_npm_catalog.py", source_metadata)
                self.assertIn("go generate ./plugin/ibm.d/modules/...", source_metadata)
                self.assertLess(names.index("Generate Source Metadata"), names.index("Generate Integrations"))
                runtime_gate = by_name["Verify generated runtime outputs"]["run"]
                self.assertIn("git diff --exit-code", runtime_gate)
                self.assertIn("git ls-files --error-unmatch", runtime_gate)
                self.assertIn("find src/go/plugin/ibm.d/modules -name module.yaml", runtime_gate)
                self.assertLess(
                    names.index("Generate Source Metadata"),
                    names.index("Verify generated runtime outputs"),
                )
                self.assertLess(
                    names.index("Verify generated runtime outputs"),
                    names.index("Generate Integrations"),
                )
                self.assertLess(
                    names.index("Generate Integrations"),
                    names.index("Generate Integrations Documentation"),
                )
                self.assertLess(
                    names.index("Generate Integrations"),
                    names.index("Generate src/collectors/SERVICE-DISCOVERY.md"),
                )
                self.assertTrue(any(step.get("uses") == "actions/setup-go@v7" for step in steps))

                environment = "virtualenv" if workflow.name == "generate-integrations.yml" else "venv"
                documentation_steps = {
                    "Generate Integrations Documentation": "python3 integrations/gen_docs_integrations.py",
                    "Generate src/collectors/COLLECTORS.md": "python3 integrations/gen_doc_collector_page.py",
                    "Generate src/collectors/SECRETS.md": "python3 integrations/gen_doc_secrets_page.py",
                    "Generate src/collectors/SERVICE-DISCOVERY.md": (
                        "python3 integrations/gen_doc_service_discovery_page.py"
                    ),
                }
                for step_name, command in documentation_steps.items():
                    run = by_name[step_name]["run"]
                    self.assertIn(
                        f"source ./{environment}/bin/activate",
                        run,
                        step_name,
                    )
                    self.assertIn(command, run, step_name)

                if workflow.name == "check-markdown.yml":
                    description_test = by_name["Test Integration Descriptions"]["run"]
                    self.assertIn("LEARN_INGEST_PATH=../learn/ingest/ingest.py", description_test)

        post_merge_data = yaml.load(workflows[1].read_text(encoding="utf-8"))
        post_merge_steps = next(iter(post_merge_data["jobs"].values()))["steps"]
        post_merge_by_name = {step["name"]: step for step in post_merge_steps}
        cleanup = post_merge_by_name["Clean Up Temporary Data"]["run"]
        self.assertIn("src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt", cleanup)
        self.assertEqual(post_merge_by_name["Create PR"]["uses"], "peter-evans/create-pull-request@v8")

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

    def test_libnetdata_reference_pages_have_owned_content_and_valid_source_links(self):
        for relative_path, required_links in LIBNETDATA_REFERENCE_LINKS.items():
            page_path = REPO_ROOT / relative_path
            markdown = page_path.read_text(encoding="utf-8")
            self.assertEqual(top_level_heading_count(markdown), 1, relative_path)
            self.assertRegex(markdown, r"(?m)^## ", relative_path)

            link_targets = set(re.findall(r"\[[^]]+\]\(([^)]+)\)", markdown))
            source_url_prefix = (
                "https://github.com/netdata/netdata/blob/master/"
                f"{page_path.parent.relative_to(REPO_ROOT).as_posix()}/"
            )
            source_links = {target for target in link_targets if target.startswith(source_url_prefix)}
            linked_files = {target.removeprefix(source_url_prefix) for target in source_links}
            self.assertTrue(required_links.issubset(linked_files), relative_path)
            for linked_file in linked_files:
                target_path = (page_path.parent / linked_file).resolve()
                self.assertTrue(target_path.is_relative_to(REPO_ROOT), f"{relative_path}: {linked_file}")
                self.assertTrue(target_path.is_file(), f"{relative_path}: {linked_file}")


if __name__ == "__main__":
    unittest.main()
