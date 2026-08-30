#!/usr/bin/env python3

from __future__ import print_function

import importlib.util
import ipaddress
import io
import os
import signal
import stat
# Signal cleanup tests spawn controlled helper scripts.
import subprocess  # nosec B404
import sys
import tarfile
import tempfile
import time
import types
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest import mock


SCRIPT_PATH = os.path.join(os.path.dirname(__file__), "collect-snmp-troubleshooting-data.py")
SPEC = importlib.util.spec_from_file_location("snmp_collector", SCRIPT_PATH)
collector = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(collector)


def arguments(**overrides):
    values = {
        "snmp_version": "2c",
        "port": 161,
        "timeout": 5,
        "retries": 1,
        "v3_user": None,
        "v3_level": "authPriv",
        "v3_auth_proto": "sha512",
        "v3_priv_proto": "aes",
        "v3_context": "",
    }
    values.update(overrides)
    return types.SimpleNamespace(**values)


class TemporaryDirectoryTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.temporary.cleanup()

    def path(self, name):
        return os.path.join(self.temporary.name, name)

    def write(self, name, content):
        path = self.path(name)
        with open(path, "w") as handle:
            handle.write(content)
        return path


class ConfigurationTests(TemporaryDirectoryTest):
    def test_explicit_config_wins(self):
        explicit = self.write("explicit.conf", "jobs: []\n")
        first = self.path("first")
        self.assertEqual(collector.select_netdata_configs(explicit, (first,)), [explicit])

    def test_missing_explicit_config_is_an_error(self):
        missing = self.path("missing.conf")
        with self.assertRaisesRegex(collector.CollectionError, "does not exist"):
            collector.select_netdata_configs(missing, ())

    def test_empty_explicit_config_path_is_an_error(self):
        with self.assertRaisesRegex(collector.CollectionError, "non-empty FILE"):
            collector.select_netdata_configs("", ())

    def test_first_default_config_wins(self):
        first = self.path("first-root")
        second = self.path("second-root")
        os.makedirs(os.path.join(first, "go.d", "sd"))
        os.makedirs(os.path.join(second, "go.d", "sd"))
        first_static = os.path.join(first, "go.d", "snmp.conf")
        first_discovery = os.path.join(first, "go.d", "sd", "snmp.conf")
        second_static = os.path.join(second, "go.d", "snmp.conf")
        for path in (first_static, first_discovery, second_static):
            with open(path, "w") as handle:
                handle.write("jobs: []\n")
        self.assertEqual(
            collector.select_netdata_configs(None, (first, second)),
            [first_static, first_discovery],
        )

    def test_no_config_is_allowed(self):
        self.assertEqual(
            collector.select_netdata_configs(None, (self.path("one"), self.path("two"))), []
        )

    def test_reads_v2c_and_v3_jobs_and_skips_disabled_jobs(self):
        path = self.write(
            "snmp.conf",
            """
jobs:
  - name: access-switch
    hostname: 192.0.2.10
    community: private
    options:
      version: 2
      port: 1161
      timeout: 7
      retries: 2
  - name: core-router
    hostname: 2001:db8::10
    options:
      version: 3
    user:
      name: operator
      level: authNoPriv
      auth_proto: sha256
      auth_key: secret
  - name: disabled
    hostname: 192.0.2.20
    enabled: false
""",
        )
        nodes = collector.read_netdata_jobs(path)
        self.assertEqual([node.name for node in nodes], ["access-switch", "core-router"])
        self.assertEqual(nodes[0].version, "2c")
        self.assertEqual(nodes[0].port, 1161)
        self.assertEqual(nodes[0].timeout, 7)
        self.assertEqual(nodes[0].retries, 2)
        self.assertEqual(nodes[1].version, "3")
        self.assertEqual(nodes[1].level, "authNoPriv")
        self.assertEqual(nodes[1].auth_proto, "SHA-256")

    def test_reads_numeric_v3_aliases_from_static_job(self):
        path = self.write(
            "numeric-v3.conf",
            """
jobs:
  - name: numeric-v3
    hostname: 192.0.2.10
    options:
      version: 3
    user:
      name: operator
      level: 3
      auth_proto: 5
      auth_key: auth-secret
      priv_proto: 3
      priv_key: priv-secret
""",
        )
        node = collector.read_netdata_jobs(path)[0]
        self.assertEqual(node.level, "authPriv")
        self.assertEqual(node.auth_proto, "SHA-256")
        self.assertEqual(node.priv_proto, "AES")

    def test_missing_jobs_list_is_an_error(self):
        path = self.write("snmp.conf", "not_jobs: []\n")
        with self.assertRaisesRegex(collector.CollectionError, "neither a go.d SNMP jobs file"):
            collector.read_netdata_jobs(path)

    def test_null_jobs_list_is_empty(self):
        path = self.write("snmp.conf", "jobs: null\n")
        self.assertEqual(collector.read_netdata_jobs(path), [])

    def test_automatic_config_skips_empty_file_and_reads_discovery(self):
        static_path = self.write("snmp.conf", "# Use stock defaults.\n")
        discovery_path = self.write(
            "sd-snmp.conf",
            """
discoverer:
  snmp:
    credentials:
      - name: public
        version: 2c
        community: public
    networks:
      - subnet: 192.0.2.0/24
        credential: public
""",
        )
        nodes, scopes = collector.read_netdata_configuration(
            [static_path, discovery_path], skip_empty=True
        )
        self.assertEqual(nodes, [])
        self.assertEqual(len(scopes), 1)

    def test_explicit_empty_config_is_an_error(self):
        path = self.write("snmp.conf", "# Use stock defaults.\n")
        with self.assertRaisesRegex(collector.CollectionError, "neither"):
            collector.read_netdata_configuration([path])

    def test_reads_discovery_credentials_and_networks_without_creating_nodes(self):
        path = self.write(
            "sd-snmp.conf",
            """
disabled: false
discoverer:
  snmp:
    credentials:
      - name: lan-v3
        version: 3
        username: operator
        security_level: authNoPriv
        auth_protocol: sha256
        auth_password: secret
    networks:
      - subnet: 192.0.2.0/24
        credential: lan-v3
services: []
""",
        )
        nodes, scopes = collector.read_netdata_configuration([path])
        self.assertEqual(nodes, [])
        self.assertEqual(len(scopes), 1)
        self.assertEqual(scopes[0].subnet, "192.0.2.0/24")
        self.assertTrue(scopes[0].contains(ipaddress.ip_address("192.0.2.10")))
        self.assertFalse(scopes[0].contains(ipaddress.ip_address("192.0.3.10")))

    def test_disabled_discovery_file_has_no_credential_scopes(self):
        path = self.write(
            "sd-snmp.conf",
            "disabled: true\ndiscoverer:\n  snmp:\n    credentials: []\n    networks: []\n",
        )
        _nodes, scopes = collector.read_netdata_configuration([path])
        self.assertEqual(scopes, [])

    def test_discovery_range_formats_match_exact_addresses(self):
        for subnet, inside, outside in (
            ("192.0.2.10-192.0.2.20", "192.0.2.15", "192.0.2.21"),
            ("192.0.2.0/255.255.255.0", "192.0.2.10", "192.0.3.10"),
            ("2001:db8::/126", "2001:db8::1", "2001:db8::8"),
        ):
            scope = collector.DiscoveryScope(
                subnet,
                collector.parse_address_range(subnet, "test"),
                {},
                "test",
            )
            self.assertTrue(scope.contains(ipaddress.ip_address(inside)))
            self.assertFalse(scope.contains(ipaddress.ip_address(outside)))

    def test_duplicate_job_names_are_an_error(self):
        path = self.write(
            "snmp.conf",
            "jobs:\n  - {name: same, hostname: 192.0.2.1}\n"
            "  - {name: same, hostname: 192.0.2.2}\n",
        )
        with self.assertRaisesRegex(collector.CollectionError, "duplicate job name"):
            collector.read_netdata_jobs(path)

    def test_unreadable_config_explains_sudo(self):
        path = self.write("snmp.conf", "jobs: []\n")
        with mock.patch("builtins.open", side_effect=PermissionError("denied")):
            with self.assertRaisesRegex(collector.CollectionError, "sudo"):
                collector.read_netdata_jobs(path)


class SelectionTests(unittest.TestCase):
    def setUp(self):
        self.configured = [
            collector.Node("switch-a", "192.0.2.10", community="one", source="config"),
            collector.Node("switch-b", "192.0.2.11", community="two", source="config"),
        ]

    def test_job_name_reuses_configured_settings(self):
        nodes = collector.select_nodes(self.configured, [], ["switch-a"], False, arguments())
        self.assertEqual(len(nodes), 1)
        self.assertIs(nodes[0], self.configured[0])

    def test_configured_address_reuses_configured_settings(self):
        nodes = collector.select_nodes(self.configured, [], ["192.0.2.11"], False, arguments())
        self.assertIs(nodes[0], self.configured[1])

    def test_unmatched_selector_becomes_manual_node(self):
        nodes = collector.select_nodes(
            self.configured,
            [],
            ["edge=192.0.2.30"],
            False,
            arguments(snmp_version="1", port=1161),
        )
        self.assertEqual(nodes[0].name, "edge")
        self.assertEqual(nodes[0].hostname, "192.0.2.30")
        self.assertEqual(nodes[0].version, "1")
        self.assertEqual(nodes[0].port, 1161)
        self.assertEqual(nodes[0].source, "manual")

    def test_all_configured_selects_all_jobs(self):
        nodes = collector.select_nodes(self.configured, [], [], True, arguments())
        self.assertEqual(nodes, self.configured)

    def test_no_selection_is_an_error(self):
        args = arguments()
        with self.assertRaisesRegex(collector.CollectionError, "select at least one"):
            collector.select_nodes([], [], [], False, args)

    def test_all_configured_requires_jobs(self):
        args = arguments()
        with self.assertRaisesRegex(collector.CollectionError, "requires static SNMP jobs"):
            collector.select_nodes([], [], [], True, args)

    def test_ambiguous_configured_address_requires_job_name(self):
        configured = self.configured + [
            collector.Node("switch-c", "192.0.2.10", community="three", source="config")
        ]
        args = arguments()
        with self.assertRaisesRegex(collector.CollectionError, "select a job name"):
            collector.select_nodes(configured, [], ["192.0.2.10"], False, args)

    def test_exact_discovery_address_reuses_matching_credential(self):
        scope = collector.DiscoveryScope(
            "192.0.2.0/24",
            collector.parse_address_range("192.0.2.0/24", "test"),
            {"name": "lan", "version": "2c", "community": "private"},
            "/private/sd/snmp.conf",
        )
        nodes = collector.select_nodes([], [scope], ["192.0.2.10"], False, arguments())
        self.assertEqual(nodes[0].community, "private")
        self.assertIn("discovery config", nodes[0].source)

    def test_discovery_v3_omitted_security_uses_go_discoverer_defaults(self):
        scope = collector.DiscoveryScope(
            "192.0.2.0/24",
            collector.parse_address_range("192.0.2.0/24", "test"),
            {"name": "v3", "version": "3", "username": "operator"},
            "discovery.conf",
        )
        nodes = collector.select_nodes(
            [], [scope], ["192.0.2.10"], False, arguments()
        )
        self.assertEqual(nodes[0].level, "noAuthNoPriv")
        self.assertEqual(nodes[0].auth_proto, "NONE")
        self.assertEqual(nodes[0].priv_proto, "NONE")

    def test_discovery_v3_accepts_numeric_security_aliases(self):
        # Fixed synthetic values make protocol-alias behavior deterministic; they are not real credentials.
        scope = collector.DiscoveryScope(
            "192.0.2.0/24",
            collector.parse_address_range("192.0.2.0/24", "test"),
            {
                "name": "v3",
                "version": "3",
                "username": "operator",
                "security_level": 3,
                "auth_protocol": 5,
                "auth_password": "auth-secret",  # nosec B105
                "priv_protocol": 3,
                "priv_password": "priv-secret",  # nosec B105
            },
            "discovery.conf",
        )
        node = collector.select_nodes(
            [], [scope], ["192.0.2.10"], False, arguments()
        )[0]
        self.assertEqual(node.level, "authPriv")
        self.assertEqual(node.auth_proto, "SHA-256")
        self.assertEqual(node.priv_proto, "AES")

    def test_manual_v3_uses_portable_aes_default(self):
        args = collector.build_parser().parse_args(
            ["--node", "switch=192.0.2.10", "--snmp-version", "3"]
        )
        nodes = collector.select_nodes([], [], args.node, False, args)
        self.assertEqual(nodes[0].priv_proto, "AES")

    def test_name_equals_address_forces_manual_settings(self):
        scope = collector.DiscoveryScope(
            "192.0.2.0/24",
            collector.parse_address_range("192.0.2.0/24", "test"),
            {"name": "lan", "version": "2c", "community": "private"},
            "/private/sd/snmp.conf",
        )
        nodes = collector.select_nodes(
            [], [scope], ["manual=192.0.2.10"], False, arguments(snmp_version="1")
        )
        self.assertEqual(nodes[0].version, "1")
        self.assertIsNone(nodes[0].community)

    def test_overlapping_discovery_scopes_require_manual_settings(self):
        scopes = [
            collector.DiscoveryScope(
                subnet,
                collector.parse_address_range(subnet, "test"),
                {"name": subnet, "version": "2c", "community": subnet},
                "/private/sd/snmp.conf",
            )
            for subnet in ("192.0.2.0/24", "192.0.2.0/25")
        ]
        args = arguments()
        with self.assertRaisesRegex(collector.CollectionError, "multiple discovery"):
            collector.select_nodes([], scopes, ["192.0.2.10"], False, args)

    def test_identical_repeated_selector_is_collected_once(self):
        nodes = collector.select_nodes(
            self.configured, [], ["switch-a", "switch-a"], False, arguments()
        )
        self.assertEqual(nodes, [self.configured[0]])

    def test_same_name_with_different_targets_is_an_error(self):
        args = arguments()
        with self.assertRaisesRegex(collector.CollectionError, "conflicting addresses"):
            collector.select_nodes(
                [],
                [],
                ["router=192.0.2.10", "router=192.0.2.11"],
                False,
                args,
            )


class CredentialTests(TemporaryDirectoryTest):
    def test_manual_v2c_community_is_prompted(self):
        node = collector.Node("switch", "192.0.2.10")
        with mock.patch("getpass.getpass", return_value="private") as prompt:
            collector.prepare_nodes([node])
        self.assertEqual(node.community, "private")
        prompt.assert_called_once()

    def test_manual_v3_credentials_are_prompted(self):
        node = collector.Node(
            "router",
            "192.0.2.20",
            version="3",
            username="operator",
            level="authPriv",
            auth_proto="sha256",
            priv_proto="aes256",
        )
        with mock.patch("getpass.getpass", side_effect=["auth-secret", "priv-secret"]):
            collector.prepare_nodes([node])
        self.assertEqual(node.auth_key, "auth-secret")
        self.assertEqual(node.priv_key, "priv-secret")

    def test_no_colon_reference_is_literal_when_environment_variable_exists(self):
        node = collector.Node("switch", "192.0.2.10", community="prefix${SNMP_TEST_SECRET}suffix")
        # Synthetic values verify resolver behavior and are not credentials.
        with mock.patch.dict(os.environ, {"SNMP_TEST_SECRET": "replacement"}):  # nosec B105
            collector.prepare_nodes([node])
        self.assertEqual(node.community, "prefix${SNMP_TEST_SECRET}suffix")

    def test_netdata_environment_reference_is_resolved(self):
        node = collector.Node("switch", "192.0.2.10", community="${env:SNMP_TEST_SECRET}")
        with mock.patch.dict(os.environ, {"SNMP_TEST_SECRET": "  resolved  "}):  # nosec B105
            collector.prepare_nodes([node])
        self.assertEqual(node.community, "resolved")

    def test_whitespace_only_netdata_environment_reference_is_empty(self):
        node = collector.Node("switch", "192.0.2.10", community="${env:SNMP_TEST_SECRET}")
        with mock.patch.dict(os.environ, {"SNMP_TEST_SECRET": " \t\n "}):  # nosec B105
            with self.assertRaisesRegex(collector.CollectionError, "must not be empty"):
                collector.prepare_nodes([node])

    def test_dollar_characters_in_credentials_are_not_treated_as_resolvers(self):
        nodes = [
            collector.Node("raw", "192.0.2.10", community="value$with$dollars"),
            collector.Node("environment", "192.0.2.11", community="${env:SNMP_TEST_SECRET}"),
        ]
        with mock.patch.dict(os.environ, {"SNMP_TEST_SECRET": "another$value"}):  # nosec B105
            collector.prepare_nodes(nodes)
        self.assertEqual(nodes[0].community, "value$with$dollars")
        self.assertEqual(nodes[1].community, "another$value")

    def test_no_colon_reference_is_literal_when_environment_variable_is_absent(self):
        node = collector.Node("switch", "192.0.2.10", community="${NOT_DEFINED}")
        with mock.patch.dict(os.environ, {}, clear=True):
            collector.prepare_nodes([node])
        self.assertEqual(node.community, "${NOT_DEFINED}")

    def test_netdata_secret_resolver_is_prompted(self):
        node = collector.Node("switch", "192.0.2.10", community="${file:/private/community}")
        with mock.patch("getpass.getpass", return_value="entered"):
            collector.prepare_nodes([node])
        self.assertEqual(node.community, "entered")

    def test_missing_secret_without_interactive_input_is_a_clear_error(self):
        node = collector.Node("switch", "192.0.2.10", community="${env:NOT_DEFINED}")
        with mock.patch("getpass.getpass", side_effect=EOFError):
            with self.assertRaisesRegex(collector.CollectionError, "input is not interactive"):
                collector.prepare_nodes([node])

    def test_private_config_contains_credentials_but_command_does_not(self):
        node = collector.Node("switch", "192.0.2.10", community="very-secret")
        config_dir = self.path("credentials")
        os.mkdir(config_dir, 0o700)
        config_path = collector.write_private_snmp_config(config_dir, node)
        with open(config_path, "r") as handle:
            self.assertIn("very-secret", handle.read())
        self.assertEqual(stat.S_IMODE(os.stat(config_path).st_mode), 0o600)

        output_path = self.path("output.txt")
        captured = {}

        def fake_run(command, environment, output, timeout):
            captured["command"] = command
            captured["environment"] = environment
            output.write(b"raw data\n")
            return collector.CommandResult(0, False, False, 9)

        with mock.patch.object(collector, "run_bounded", side_effect=fake_run):
            with redirect_stdout(io.StringIO()):
                collector.collect_probe(
                    "/usr/bin/snmpwalk",
                    node,
                    (".1.3.6.1.2.1.1",),
                    config_dir,
                    output_path,
                    5,
                )
        self.assertNotIn("very-secret", " ".join(captured["command"]))
        self.assertEqual(captured["environment"]["SNMPCONFPATH"], config_dir)


class PlanTests(unittest.TestCase):
    def test_plan_lists_nodes_versions_and_protocols_without_estimates(self):
        nodes = [
            collector.Node("switch", "192.0.2.10", version="2c", community="private"),
            collector.Node("router", "2001:db8::1", version="3", username="operator"),
        ]
        plan = collector.format_plan(nodes)
        self.assertIn("No SNMP requests have been sent", plan)
        self.assertIn("switch", plan)
        self.assertIn("192.0.2.10:161", plan)
        self.assertIn("SNMP version: 3", plan)
        for name, _oids in collector.PROBE_GROUPS:
            self.assertIn(name, plan)
        self.assertNotIn("ETA", plan)
        self.assertNotIn("estimated", plan.lower())
        self.assertNotIn("very-secret", plan)

    def test_default_confirmation_is_rejection(self):
        with mock.patch("builtins.input", return_value=""):
            self.assertFalse(collector.approve_plan(False))

    def test_yes_confirmation_is_acceptance(self):
        with mock.patch("builtins.input", return_value="yes"):
            self.assertTrue(collector.approve_plan(False))


class CommandTests(TemporaryDirectoryTest):
    def test_ipv4_hostname_and_ipv6_targets(self):
        self.assertEqual(collector.net_snmp_target("192.0.2.1", 161), "udp:192.0.2.1:161")
        self.assertEqual(collector.net_snmp_target("switch.example", 1161), "udp:switch.example:1161")
        self.assertEqual(collector.net_snmp_target("2001:db8::1", 161), "udp6:[2001:db8::1]:161")

    def test_output_is_hard_bounded(self):
        output_path = self.path("bounded.bin")
        command = [sys.executable, "-c", "import sys; sys.stdout.write('x' * (8 * 1024 * 1024))"]
        with open(output_path, "wb") as output:
            result = collector.run_bounded(command, os.environ.copy(), output, 10, limit=1024)
        self.assertEqual(result.returncode, 0)
        self.assertTrue(result.truncated)
        self.assertEqual(os.path.getsize(output_path), 1024)

    def test_timeout_stops_the_started_process(self):
        output_path = self.path("timeout.bin")
        command = [sys.executable, "-c", "import time; time.sleep(10)"]
        with open(output_path, "wb") as output:
            result = collector.run_bounded(command, os.environ.copy(), output, 0.1)
        self.assertTrue(result.timed_out)
        self.assertIsNotNone(result.returncode)

    def test_timeout_stops_a_process_that_closed_its_output(self):
        output_path = self.path("closed-output.bin")
        command = [sys.executable, "-c", "import os,time; os.close(1); time.sleep(10)"]
        with open(output_path, "wb") as output:
            result = collector.run_bounded(command, os.environ.copy(), output, 0.1)
        self.assertTrue(result.timed_out)
        self.assertIsNotNone(result.returncode)

    def test_group_writable_snmpwalk_is_rejected(self):
        path = self.path("snmpwalk")
        with open(path, "w") as handle:
            handle.write("#!/bin/sh\n")
        # The unsafe mode is intentional: the collector must reject this fixture.
        os.chmod(path, 0o770)  # nosec B103
        with self.assertRaisesRegex(collector.CollectionError, "snmpwalk was not found"):
            collector.find_snmpwalk((path,))


class SignalCleanupTests(TemporaryDirectoryTest):
    def wait_for_child_pid(self, path, process):
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            try:
                with open(path, "r") as handle:
                    content = handle.read().strip()
            except FileNotFoundError:
                content = ""
            if content:
                try:
                    return int(content)
                except ValueError:
                    pass
            if process.poll() is not None:
                break
            time.sleep(0.05)
        self.fail("the test SNMP process did not start")

    def test_signal_during_process_spawn_terminates_the_acquired_child(self):
        real_popen = subprocess.Popen
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            with self.subTest(signal=signum):
                output_path = self.path("spawn-window-{}.bin".format(signum))
                spawned = []

                def spawn_then_signal(*args, **kwargs):
                    process = real_popen(*args, **kwargs)
                    spawned.append(process)
                    os.kill(os.getpid(), signum)
                    return process

                command = [sys.executable, "-c", "import time; time.sleep(60)"]
                environment = os.environ.copy()
                previous = collector.install_termination_handlers()
                try:
                    with mock.patch.object(collector.subprocess, "Popen", side_effect=spawn_then_signal):
                        with open(output_path, "wb") as output:
                            with self.assertRaises(collector.CollectionInterrupted):
                                collector.run_bounded(command, environment, output, 120)
                finally:
                    collector.restore_signal_handlers(previous)
                self.assertEqual(len(spawned), 1)
                self.assertIsNotNone(spawned[0].poll())

    def test_signal_during_archive_open_removes_the_acquired_file(self):
        archive_root = self.path("archive-source")
        os.mkdir(archive_root)
        collector.write_text(os.path.join(archive_root, "data.txt"), "raw data\n")
        real_open = os.open
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            with self.subTest(signal=signum):
                output_dir = self.path("archive-output-{}".format(signum))
                os.mkdir(output_dir)

                def open_then_signal(path, flags, mode=0o777):
                    descriptor = real_open(path, flags, mode)
                    if path.endswith(".tar.gz"):
                        os.kill(os.getpid(), signum)
                    return descriptor

                previous = collector.install_termination_handlers()
                try:
                    with mock.patch.object(collector.os, "open", side_effect=open_then_signal):
                        with self.assertRaises(collector.CollectionInterrupted):
                            collector.create_archive(archive_root, output_dir)
                finally:
                    collector.restore_signal_handlers(previous)
                self.assertEqual(os.listdir(output_dir), [])

    def test_signal_during_temporary_tree_creation_removes_the_tree(self):
        real_mkdtemp = tempfile.mkdtemp
        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
            with self.subTest(signal=signum):
                output_dir = self.path("temporary-output-{}".format(signum))
                os.mkdir(output_dir)

                def create_then_signal(*args, **kwargs):
                    path = real_mkdtemp(*args, **kwargs)
                    os.kill(os.getpid(), signum)
                    return path

                previous = collector.install_termination_handlers()
                try:
                    with mock.patch.object(collector.tempfile, "mkdtemp", side_effect=create_then_signal):
                        with self.assertRaises(collector.CollectionInterrupted):
                            collector.collect([], "plan", "/usr/bin/snmpwalk", output_dir, 5)
                finally:
                    collector.restore_signal_handlers(previous)
                self.assertEqual(os.listdir(output_dir), [])

    def run_signal_case(self, signum, second_signal=None, second_signal_phase=None):
        output_dir = self.path("output-{}".format(signum))
        os.mkdir(output_dir)
        child_pid_path = self.path("child-{}.pid".format(signum))
        cleanup_marker_path = self.path("cleanup-{}-{}".format(signum, second_signal_phase))
        child_signal_handler = ""
        if second_signal_phase == "child":
            child_signal_handler = (
                "import signal\n"
                "def handle_term(_signum, _frame):\n"
                "    with open(os.environ['SNMP_TEST_CLEANUP_MARKER'], 'w') as handle:\n"
                "        handle.write('terminating')\n"
                "signal.signal(signal.SIGTERM, handle_term)\n"
            )
        fake_snmpwalk = self.write(
            "snmpwalk-{}.py".format(signum),
            "#!{}\n"
            "import os\n"
            "import time\n"
            "{}"
            "with open(os.environ['SNMP_TEST_CHILD_PID'], 'w') as handle:\n"
            "    handle.write(str(os.getpid()))\n"
            "time.sleep(60)\n".format(sys.executable, child_signal_handler),
        )
        os.chmod(fake_snmpwalk, 0o700)
        slow_tree_cleanup = ""
        if second_signal_phase == "tree":
            slow_tree_cleanup = (
                "real_rmtree = collector.shutil.rmtree\n"
                "def slow_rmtree(*args, **kwargs):\n"
                "    with open({!r}, 'w') as handle:\n"
                "        handle.write('removing')\n"
                "    time.sleep(1)\n"
                "    return real_rmtree(*args, **kwargs)\n"
                "collector.shutil.rmtree = slow_rmtree\n".format(cleanup_marker_path)
            )
        driver = self.write(
            "driver-{}.py".format(signum),
            "import importlib.util\n"
            "import sys\n"
            "import time\n"
            "spec = importlib.util.spec_from_file_location('collector', {!r})\n"
            "collector = importlib.util.module_from_spec(spec)\n"
            "spec.loader.exec_module(collector)\n"
            "collector.PROBE_GROUPS = (('system', ('.1',)),)\n"
            "{}"
            "previous = collector.install_termination_handlers()\n"
            "try:\n"
            "    node = collector.Node('switch', '192.0.2.10', community='private')\n"
            "    collector.collect([node], collector.format_plan([node]), {!r}, {!r}, 120)\n"
            "except collector.CollectionInterrupted as interruption:\n"
            "    sys.exit(128 + interruption.signum)\n"
            "finally:\n"
            "    collector.restore_signal_handlers(previous)\n".format(
                SCRIPT_PATH, slow_tree_cleanup, fake_snmpwalk, output_dir
            ),
        )
        environment = os.environ.copy()
        environment["SNMP_TEST_CHILD_PID"] = child_pid_path
        environment["SNMP_TEST_CLEANUP_MARKER"] = cleanup_marker_path
        # The generated driver path and arguments are controlled by this test.
        process = subprocess.Popen(  # nosec B603
            [sys.executable, driver],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=environment,
        )
        child_pid = None
        try:
            child_pid = self.wait_for_child_pid(child_pid_path, process)

            os.kill(process.pid, signum)
            if second_signal is not None:
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline and not os.path.exists(cleanup_marker_path):
                    if process.poll() is not None:
                        break
                    time.sleep(0.05)
                self.assertTrue(
                    os.path.exists(cleanup_marker_path),
                    "cleanup did not reach the requested second-signal window",
                )
                os.kill(process.pid, second_signal)
            stdout, stderr = process.communicate(timeout=10)
            self.assertEqual(
                process.returncode,
                128 + signum,
                "driver output:\n{}\n{}".format(
                    stdout.decode("utf-8", "replace"), stderr.decode("utf-8", "replace")
                ),
            )
            with self.assertRaises(ProcessLookupError):
                os.kill(child_pid, 0)
            leftovers = [name for name in os.listdir(output_dir) if name.startswith("netdata-snmp-")]
            self.assertEqual(leftovers, [])
        finally:
            if process.poll() is None:
                os.kill(process.pid, signal.SIGKILL)
                process.wait()
            if child_pid is not None:
                try:
                    os.kill(child_pid, 0)
                except ProcessLookupError:
                    pass
                else:
                    os.kill(child_pid, signal.SIGKILL)

    def test_sighup_cleans_child_and_credentials(self):
        self.run_signal_case(signal.SIGHUP)

    def test_sigterm_cleans_child_and_credentials(self):
        self.run_signal_case(signal.SIGTERM)

    def test_sigint_cleans_child_and_credentials(self):
        self.run_signal_case(signal.SIGINT)

    def test_second_signal_during_resistant_child_cleanup_is_ignored(self):
        self.run_signal_case(signal.SIGTERM, signal.SIGHUP, "child")

    def test_second_signal_during_credential_tree_cleanup_is_ignored(self):
        self.run_signal_case(signal.SIGHUP, signal.SIGTERM, "tree")

    def test_second_signal_after_interrupted_cli_main_preserves_first_exit(self):
        output_dir = self.path("cli-exit-output")
        os.mkdir(output_dir)
        child_pid_path = self.path("cli-exit-child.pid")
        exit_marker_path = self.path("cli-exit.marker")
        fake_snmpwalk = self.write(
            "cli-exit-snmpwalk.py",
            "#!{}\n"
            "import os\n"
            "import time\n"
            "with open(os.environ['SNMP_TEST_CHILD_PID'], 'w') as handle:\n"
            "    handle.write(str(os.getpid()))\n"
            "time.sleep(60)\n".format(sys.executable),
        )
        os.chmod(fake_snmpwalk, 0o700)
        driver = self.write(
            "cli-exit-driver.py",
            "import importlib.util\n"
            "import sys\n"
            "import time\n"
            "spec = importlib.util.spec_from_file_location('collector', {!r})\n"
            "collector = importlib.util.module_from_spec(spec)\n"
            "spec.loader.exec_module(collector)\n"
            "collector.PROBE_GROUPS = (('system', ('.1',)),)\n"
            "collector.find_snmpwalk = lambda: {!r}\n"
            "collector.getpass.getpass = lambda _prompt: 'private'\n"
            "sys.argv = ['collector', '--node', 'switch=192.0.2.10', '--yes', "
            "'--output-dir', {!r}]\n"
            "result = collector.main()\n"
            "with open({!r}, 'w') as handle:\n"
            "    handle.write(str(result))\n"
            "time.sleep(1)\n"
            "sys.exit(result)\n".format(
                SCRIPT_PATH, fake_snmpwalk, output_dir, exit_marker_path
            ),
        )
        environment = os.environ.copy()
        environment["SNMP_TEST_CHILD_PID"] = child_pid_path
        # The generated driver path and arguments are controlled by this test.
        process = subprocess.Popen(  # nosec B603
            [sys.executable, driver],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=environment,
        )
        child_pid = None
        try:
            child_pid = self.wait_for_child_pid(child_pid_path, process)

            os.kill(process.pid, signal.SIGINT)
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline and not os.path.exists(exit_marker_path):
                if process.poll() is not None:
                    break
                time.sleep(0.05)
            self.assertTrue(os.path.exists(exit_marker_path), "main did not reach its interrupted return")
            with open(exit_marker_path, "r") as handle:
                self.assertEqual(handle.read(), "130")

            os.kill(process.pid, signal.SIGHUP)
            stdout, stderr = process.communicate(timeout=10)
            self.assertEqual(
                process.returncode,
                130,
                "driver output:\n{}\n{}".format(
                    stdout.decode("utf-8", "replace"), stderr.decode("utf-8", "replace")
                ),
            )
            with self.assertRaises(ProcessLookupError):
                os.kill(child_pid, 0)
            leftovers = [name for name in os.listdir(output_dir) if name.startswith("netdata-snmp-")]
            self.assertEqual(leftovers, [])
        finally:
            if process.poll() is None:
                os.kill(process.pid, signal.SIGKILL)
                process.wait()
            if child_pid is not None:
                try:
                    os.kill(child_pid, 0)
                except ProcessLookupError:
                    pass
                else:
                    os.kill(child_pid, signal.SIGKILL)


class ArchiveTests(TemporaryDirectoryTest):
    def test_collection_continues_and_archive_excludes_credentials(self):
        node = collector.Node("switch", "192.0.2.10", community="archive-secret")

        def fake_probe(_tool, _node, oids, _config, output_path, _timeout):
            group_name = "failed-protocol" if oids == (".2",) else "successful-protocol"
            collector.write_text(output_path, "raw {}\n".format(group_name))
            if group_name == "failed-protocol":
                return "failed", "test failure"
            return "success", "collected"

        groups = (("successful-protocol", (".1",)), ("failed-protocol", (".2",)))
        with mock.patch.object(collector, "PROBE_GROUPS", groups):
            with mock.patch.object(collector, "collect_probe", side_effect=fake_probe):
                with redirect_stdout(io.StringIO()):
                    rows, summary, archive_path = collector.collect(
                        [node], collector.format_plan([node]), "/usr/bin/snmpwalk", self.temporary.name, 5
                    )

        self.assertEqual([row[3] for row in rows], ["success", "failed"])
        self.assertIn("Successful protocol collections: 1", summary)
        self.assertIn("Failed protocol collections: 1", summary)
        self.assertEqual(stat.S_IMODE(os.stat(archive_path).st_mode), 0o600)

        with tarfile.open(archive_path, "r:gz") as archive:
            names = archive.getnames()
            self.assertIn("plan.txt", names)
            self.assertIn("collection-status.tsv", names)
            self.assertIn("summary.txt", names)
            self.assertIn("README.txt", names)
            self.assertIn("manifest.sha256", names)
            self.assertFalse(any("credential" in name or name.endswith("snmp.conf") for name in names))
            for member in archive.getmembers():
                if member.isfile():
                    self.assertNotIn(b"archive-secret", archive.extractfile(member).read())


class MainTests(TemporaryDirectoryTest):
    def test_no_arguments_prints_help(self):
        output = io.StringIO()
        with redirect_stdout(output):
            self.assertEqual(collector.main([]), 0)
        self.assertIn("--node", output.getvalue())
        self.assertIn("--netdata-snmp-config", output.getvalue())

    def test_missing_explicit_config_fails_even_with_manual_node(self):
        error = io.StringIO()
        with redirect_stderr(error):
            code = collector.main(
                [
                    "--node",
                    "192.0.2.10",
                    "--netdata-snmp-config",
                    self.path("missing.conf"),
                ]
            )
        self.assertEqual(code, 1)
        self.assertIn("does not exist", error.getvalue())

    def test_empty_explicit_config_fails_before_manual_plan(self):
        output = io.StringIO()
        error = io.StringIO()
        with redirect_stdout(output), redirect_stderr(error):
            code = collector.main(
                [
                    "--node",
                    "manual=192.0.2.10",
                    "--netdata-snmp-config",
                    "",
                ]
            )
        self.assertEqual(code, 1)
        self.assertNotIn("SNMP data collection plan", output.getvalue())
        self.assertIn("non-empty FILE", error.getvalue())

    def test_manual_only_selection_bypasses_unusable_automatic_config(self):
        for failure in ("unreadable automatic config", "malformed automatic config"):
            with self.subTest(failure=failure):
                with mock.patch.object(
                    collector, "select_netdata_configs", side_effect=collector.CollectionError(failure)
                ) as select_config:
                    with mock.patch.object(collector, "prepare_nodes"):
                        with mock.patch.object(
                            collector, "find_snmpwalk", return_value="/usr/bin/snmpwalk"
                        ):
                            with mock.patch.object(collector, "approve_plan", return_value=False):
                                with redirect_stdout(io.StringIO()):
                                    code = collector.main(["--node", "manual=192.0.2.10"])
                self.assertEqual(code, 0)
                select_config.assert_not_called()

    def test_rejected_plan_never_collects(self):
        node = collector.Node("switch", "192.0.2.10", community="private")
        with mock.patch.object(collector, "select_netdata_configs", return_value=[]):
            with mock.patch.object(collector, "select_nodes", return_value=[node]):
                with mock.patch.object(collector, "prepare_nodes"):
                    with mock.patch.object(collector, "find_snmpwalk", return_value="/usr/bin/snmpwalk"):
                        with mock.patch.object(collector, "approve_plan", return_value=False):
                            with mock.patch.object(collector, "collect") as collect_mock:
                                with redirect_stdout(io.StringIO()):
                                    code = collector.main(["--node", "192.0.2.10"])
        self.assertEqual(code, 0)
        collect_mock.assert_not_called()

    def test_partial_collection_returns_two_and_prints_archive(self):
        node = collector.Node("switch", "192.0.2.10", community="private")
        rows = [("switch", "192.0.2.10", "system", "failed", "timeout")]
        summary = collector.summarize(rows)
        archive_path = self.path("result.tar.gz")
        with mock.patch.object(collector, "select_netdata_configs", return_value=[]):
            with mock.patch.object(collector, "select_nodes", return_value=[node]):
                with mock.patch.object(collector, "prepare_nodes"):
                    with mock.patch.object(collector, "find_snmpwalk", return_value="/usr/bin/snmpwalk"):
                        with mock.patch.object(collector, "collect", return_value=(rows, summary, archive_path)):
                            output = io.StringIO()
                            with redirect_stdout(output):
                                code = collector.main(["--node", "192.0.2.10", "--yes"])
        self.assertEqual(code, 2)
        self.assertIn("Archive: {}".format(archive_path), output.getvalue())
        self.assertIn("Freshdesk", output.getvalue())


if __name__ == "__main__":
    unittest.main()
