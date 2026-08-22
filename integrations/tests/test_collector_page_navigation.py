#!/usr/bin/env python3

import re
import sys
import unittest
from pathlib import Path

INTEGRATIONS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(INTEGRATIONS_DIR))

from gen_doc_collector_page import _render_tech_navigation  # noqa: E402


class CollectorPageNavigationTest(unittest.TestCase):
    def test_quick_navigation_targets_generated_sections(self):
        links = re.findall(r"\[([^]]+)\]\((#[^)]+)\)", _render_tech_navigation())

        self.assertEqual(
            links,
            [
                ("AWS", "#cloud-and-devops"),
                ("Azure", "#cloud-and-devops"),
                ("GCP", "#cloud-and-devops"),
                ("Kubernetes", "#containers-and-vms"),
                ("Docker", "#containers-and-vms"),
                ("VMware", "#containers-and-vms"),
                ("MySQL", "#databases"),
                ("PostgreSQL", "#databases"),
                ("MongoDB", "#databases"),
                ("Redis", "#databases"),
                ("Elasticsearch", "#databases"),
                ("Oracle", "#databases"),
                ("NGINX", "#web-servers-and-proxies"),
                ("Apache", "#web-servers-and-proxies"),
                ("HAProxy", "#web-servers-and-proxies"),
                ("Tomcat", "#web-servers-and-proxies"),
                ("PHP-FPM", "#web-servers-and-proxies"),
                ("Kafka", "#databases"),
                ("RabbitMQ", "#databases"),
                ("ActiveMQ", "#databases"),
                ("NATS", "#databases"),
                ("Pulsar", "#databases"),
                ("Linux", "#operating-systems"),
                ("Windows", "#operating-systems"),
                ("macOS", "#operating-systems"),
                ("FreeBSD", "#operating-systems"),
                ("Prometheus endpoints", "#beyond-the-850-integrations"),
                ("SNMP devices", "#networking"),
                ("StatsD", "#beyond-the-850-integrations"),
                ("custom data sources", "#beyond-the-850-integrations"),
            ],
        )


if __name__ == "__main__":
    unittest.main()
