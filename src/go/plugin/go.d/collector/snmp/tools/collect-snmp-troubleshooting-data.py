#!/usr/bin/env python3
"""Collect bounded raw SNMP data for Netdata troubleshooting."""

from __future__ import print_function

import argparse
import errno
import fcntl
import getpass
import hashlib
import ipaddress
import os
import re
import select
import shlex
import shutil
import signal
import stat
# Required for shell-free snmpwalk execution.
import subprocess  # nosec B404
import sys
import tarfile
import tempfile
import time
from contextlib import contextmanager


VERSION = "1.0"
DEFAULT_CONFIG_ROOTS = (
    "/etc/netdata",
    "/opt/netdata/etc/netdata",
)
SNMPWALK_PATHS = ("/usr/bin/snmpwalk", "/usr/local/bin/snmpwalk")
SNMP_CONFIG_FILENAME = "snmp.conf"
MAX_COMMAND_OUTPUT = 4 * 1024 * 1024

# These are raw MIB subtrees used to understand device identity, interfaces,
# neighbors, and the routing data from which network topology is derived.
PROBE_GROUPS = (
    ("system", (".1.3.6.1.2.1.1",)),
    ("interfaces", (".1.3.6.1.2.1.2.2.1", ".1.3.6.1.2.1.31.1.1.1")),
    ("bridge-stp-fdb", (".1.3.6.1.2.1.17",)),
    ("lldp", (".1.0.8802.1.1.2",)),
    ("lldpv2", (".1.3.111.2.802.1.1.13",)),
    ("cdp", (".1.3.6.1.4.1.9.9.23",)),
    (
        "ip-addresses-neighbors",
        (
            ".1.3.6.1.2.1.4.20",
            ".1.3.6.1.2.1.4.22",
            ".1.3.6.1.2.1.4.34.1",
            ".1.3.6.1.2.1.4.35.1",
        ),
    ),
    ("ospf", (".1.3.6.1.2.1.14",)),
    ("bgp", (".1.3.6.1.2.1.15",)),
    (
        "vendor-bgp",
        (
            ".1.3.6.1.4.1.9.9.187",        # Cisco BGP4-MIB
            ".1.3.6.1.4.1.2636.5.1.1",    # Juniper BGP4-V2-MIB
            ".1.3.6.1.4.1.2011.5.25.177",  # Huawei BGP4-V2-MIB
            ".1.3.6.1.4.1.30065.4.1",     # Arista BGP4-V2-MIB
            ".1.3.6.1.4.1.6527.3.1.2.14",  # Nokia TIMETRA-BGP-MIB
            ".1.3.6.1.4.1.6027.3.26",     # Dell FORCE10-BGP4-V2-MIB
        ),
    ),
)


class CollectionError(Exception):
    """A user-actionable collection error."""


class CollectionInterrupted(BaseException):
    """A termination signal that per-probe Exception handling must not swallow."""

    def __init__(self, signum):
        """Record the signal that initiated interruption cleanup."""
        super(CollectionInterrupted, self).__init__(signum)
        self.signum = signum


_TERMINATION_DEFER_DEPTH = 0
_PENDING_TERMINATION_SIGNAL = None
_TERMINATION_IN_PROGRESS = False


class Node(object):
    def __init__(
        self,
        name,
        hostname,
        version="2c",
        port=161,
        timeout=5,
        retries=1,
        source="manual",
        community=None,
        username=None,
        level=None,
        auth_proto=None,
        auth_key=None,
        priv_proto=None,
        priv_key=None,
        context_name="",
    ):
        """Validate and store one SNMP node configuration."""
        self.name = str(name)
        self.hostname = str(hostname)
        self.version = normalize_version(version)
        self.port = positive_int(port, "SNMP port")
        self.timeout = positive_int(timeout, "SNMP timeout")
        self.retries = nonnegative_int(retries, "SNMP retries")
        self.source = source
        self.community = scalar(community)
        self.username = scalar(username)
        self.level = normalize_level(level)
        self.auth_proto = normalize_auth_proto(auth_proto)
        self.auth_key = scalar(auth_key)
        self.priv_proto = normalize_priv_proto(priv_proto)
        self.priv_key = scalar(priv_key)
        self.context_name = scalar(context_name) or ""


class CommandResult(object):
    def __init__(self, returncode, timed_out, truncated, bytes_written):
        """Store the bounded subprocess outcome."""
        self.returncode = returncode
        self.timed_out = timed_out
        self.truncated = truncated
        self.bytes_written = bytes_written


class DiscoveryScope(object):
    def __init__(self, subnet, address_range, credential, source):
        """Store one discovery range and its referenced credential."""
        self.subnet = subnet
        self.address_range = address_range
        self.credential = credential
        self.source = source

    def contains(self, address):
        version, start, end = self.address_range
        return address.version == version and start <= int(address) <= end


def scalar(value):
    if value is None:
        return None
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value)


def positive_int(value, label):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        raise CollectionError("{} must be an integer".format(label))
    if parsed < 1:
        raise CollectionError("{} must be at least 1".format(label))
    return parsed


def nonnegative_int(value, label):
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        raise CollectionError("{} must be an integer".format(label))
    if parsed < 0:
        raise CollectionError("{} must not be negative".format(label))
    return parsed


def normalize_version(value):
    mapping = {"0": "1", "1": "1", "2": "2c", "2c": "2c", "3": "3"}
    version = mapping.get(str(value).lower())
    if version is None:
        raise CollectionError("unsupported SNMP version {!r}; use 1, 2c, or 3".format(value))
    return version


def normalize_level(value):
    mapping = {
        None: "noAuthNoPriv",
        "": "noAuthNoPriv",
        "1": "noAuthNoPriv",
        "none": "noAuthNoPriv",
        "noauthnopriv": "noAuthNoPriv",
        "2": "authNoPriv",
        "authnopriv": "authNoPriv",
        "3": "authPriv",
        "authpriv": "authPriv",
    }
    level = mapping.get(None if value is None else str(value).lower())
    if level is None:
        raise CollectionError(
            "unsupported SNMPv3 security level {!r}; use none, authNoPriv, or authPriv".format(value)
        )
    return level


def normalize_auth_proto(value):
    mapping = {
        None: "NONE",
        "": "NONE",
        "1": "NONE",
        "none": "NONE",
        "2": "MD5",
        "md5": "MD5",
        "3": "SHA",
        "sha": "SHA",
        "4": "SHA-224",
        "sha224": "SHA-224",
        "5": "SHA-256",
        "sha256": "SHA-256",
        "6": "SHA-384",
        "sha384": "SHA-384",
        "7": "SHA-512",
        "sha512": "SHA-512",
    }
    protocol = mapping.get(None if value is None else str(value).lower())
    if protocol is None:
        raise CollectionError("unsupported SNMPv3 authentication protocol {!r}".format(value))
    return protocol


def normalize_priv_proto(value):
    mapping = {
        None: "NONE",
        "": "NONE",
        "1": "NONE",
        "none": "NONE",
        "2": "DES",
        "des": "DES",
        "3": "AES",
        "aes": "AES",
        "4": "AES-192",
        "aes192": "AES-192",
        "5": "AES-256",
        "aes256": "AES-256",
        "6": "AES-192-C",
        "aes192c": "AES-192-C",
        "7": "AES-256-C",
        "aes256c": "AES-256-C",
    }
    protocol = mapping.get(None if value is None else str(value).lower())
    if protocol is None:
        raise CollectionError("unsupported SNMPv3 privacy protocol {!r}".format(value))
    return protocol


def load_yaml_module():
    try:
        import yaml
    except ImportError:
        raise CollectionError(
            "PyYAML is required to read Netdata configuration; install python3-yaml "
            "or run with only manual --node targets"
        )
    return yaml


def select_netdata_configs(explicit_path, default_roots=DEFAULT_CONFIG_ROOTS):
    if explicit_path is not None:
        if not explicit_path:
            raise CollectionError("--netdata-snmp-config requires a non-empty FILE")
        path = os.path.abspath(explicit_path)
        if not os.path.isfile(path):
            raise CollectionError("--netdata-snmp-config does not exist or is not a file: {}".format(path))
        return [path]
    for root in default_roots:
        paths = (
            os.path.join(root, "go.d", SNMP_CONFIG_FILENAME),
            os.path.join(root, "go.d", "sd", SNMP_CONFIG_FILENAME),
        )
        existing = [path for path in paths if os.path.isfile(path)]
        if existing:
            return existing
    return []


def read_yaml_document(path):
    yaml = load_yaml_module()
    try:
        with open(path, "r") as handle:
            data = yaml.safe_load(handle)
    except PermissionError:
        raise CollectionError(
            "cannot read {}. Run this command with sudo, or copy the file to a private readable path "
            "and pass --netdata-snmp-config".format(path)
        )
    except OSError as error:
        raise CollectionError("cannot read {}: {}".format(path, error))
    except yaml.YAMLError as error:
        raise CollectionError("cannot parse {} as YAML: {}".format(path, error))
    if data is None:
        return {}
    if not isinstance(data, dict):
        raise CollectionError("{} must contain a top-level YAML mapping".format(path))
    return data


def parse_static_job(job, index, path):
    if not isinstance(job, dict):
        raise CollectionError("{}: jobs[{}] must be a mapping".format(path, index))
    if job.get("enabled") is False or job.get("disabled") is True:
        return None
    name = scalar(job.get("name"))
    hostname = scalar(job.get("hostname"))
    if not name or not hostname:
        raise CollectionError("{}: jobs[{}] requires name and hostname".format(path, index))
    options = job.get("options") or {}
    user = job.get("user") or {}
    if not isinstance(options, dict) or not isinstance(user, dict):
        raise CollectionError("{}: job {!r} has invalid options or user settings".format(path, name))
    return Node(
        name=name,
        hostname=hostname,
        version=options.get("version", "2c"),
        port=options.get("port", 161),
        timeout=options.get("timeout", 5),
        retries=options.get("retries", 1),
        source="Netdata config: {}".format(path),
        community=job.get("community", "public"),
        username=user.get("name"),
        level=user.get("level", "authPriv"),
        auth_proto=user.get("auth_proto", "sha512"),
        auth_key=user.get("auth_key"),
        priv_proto=user.get("priv_proto", "aes192c"),
        priv_key=user.get("priv_key"),
        context_name=user.get("context_name", ""),
    )


def parse_static_jobs(data, path):
    jobs = data.get("jobs")
    if "jobs" not in data or jobs is None:
        return []
    if not isinstance(jobs, list):
        raise CollectionError("{}: 'jobs' must be a list or null".format(path))

    nodes = []
    seen_names = set()
    for index, job in enumerate(jobs, 1):
        node = parse_static_job(job, index, path)
        if node is None:
            continue
        if node.name in seen_names:
            raise CollectionError("{} contains duplicate job name {!r}".format(path, node.name))
        seen_names.add(node.name)
        nodes.append(node)
    return nodes


def parse_address_range(value, path):
    text = scalar(value)
    if not text:
        raise CollectionError("{}: discovery network requires a subnet".format(path))
    try:
        if "-" in text:
            first, last = text.split("-", 1)
            start = ipaddress.ip_address(first.strip())
            end = ipaddress.ip_address(last.strip())
            if start.version != end.version or int(start) > int(end):
                raise ValueError("invalid address range")
            return start.version, int(start), int(end)
        if "/" in text:
            network = ipaddress.ip_network(text, strict=False)
            start = int(network.network_address)
            end = int(network.broadcast_address)
            if (network.version == 4 and network.prefixlen < 31) or (
                network.version == 6 and network.prefixlen < 127
            ):
                start += 1
                end -= 1
            return network.version, start, end
        address = ipaddress.ip_address(text)
        return address.version, int(address), int(address)
    except ValueError as error:
        raise CollectionError(
            "{}: unsupported discovery subnet {!r}: {}".format(path, text, error)
        )


def index_discovery_credentials(credentials, path):
    by_name = {}
    for index, credential in enumerate(credentials, 1):
        if not isinstance(credential, dict):
            raise CollectionError("{}: discovery credential {} must be a mapping".format(path, index))
        name = scalar(credential.get("name"))
        if not name:
            raise CollectionError("{}: discovery credential {} requires a name".format(path, index))
        if name in by_name:
            raise CollectionError("{} contains duplicate discovery credential {!r}".format(path, name))
        by_name[name] = credential
    return by_name


def build_discovery_scopes(networks, credentials, path):
    scopes = []
    for index, network in enumerate(networks, 1):
        if not isinstance(network, dict):
            raise CollectionError("{}: discovery network {} must be a mapping".format(path, index))
        subnet = scalar(network.get("subnet"))
        credential_name = scalar(network.get("credential"))
        if credential_name not in credentials:
            raise CollectionError(
                "{}: discovery network {!r} references unknown credential {!r}".format(
                    path, subnet, credential_name
                )
            )
        scopes.append(
            DiscoveryScope(
                subnet,
                parse_address_range(subnet, path),
                credentials[credential_name],
                path,
            )
        )
    return scopes


def parse_discovery_scopes(data, path):
    discoverer = data.get("discoverer")
    if not isinstance(discoverer, dict) or "snmp" not in discoverer or data.get("disabled") is True:
        return []
    snmp = discoverer.get("snmp") or {}
    if not isinstance(snmp, dict):
        raise CollectionError("{}: discoverer.snmp must be a mapping".format(path))
    credentials = snmp.get("credentials") or []
    networks = snmp.get("networks") or []
    if not isinstance(credentials, list) or not isinstance(networks, list):
        raise CollectionError("{}: discovery credentials and networks must be lists".format(path))
    return build_discovery_scopes(networks, index_discovery_credentials(credentials, path), path)


def read_netdata_configuration(paths, skip_empty=False):
    nodes = []
    scopes = []
    seen_names = set()
    for path in paths:
        data = read_yaml_document(path)
        if not data and skip_empty:
            continue
        if "jobs" not in data and not (
            isinstance(data.get("discoverer"), dict) and "snmp" in data["discoverer"]
        ):
            raise CollectionError(
                "{} is neither a go.d SNMP jobs file nor an SNMP discovery file".format(path)
            )
        for node in parse_static_jobs(data, path):
            if node.name in seen_names:
                raise CollectionError("Netdata configuration contains duplicate job name {!r}".format(node.name))
            seen_names.add(node.name)
            nodes.append(node)
        scopes.extend(parse_discovery_scopes(data, path))
    return nodes, scopes


def read_netdata_jobs(path):
    nodes, _scopes = read_netdata_configuration([path])
    return nodes


def parse_manual_selector(selector):
    if "=" in selector:
        name, hostname = selector.split("=", 1)
        name = name.strip()
        hostname = hostname.strip()
        if not name or not hostname:
            raise CollectionError("manual --node NAME=ADDRESS requires both values")
        return name, hostname
    value = selector.strip()
    if not value:
        raise CollectionError("--node must not be empty")
    return value, value


def node_from_discovery(name, hostname, scope, args):
    credential = scope.credential
    return Node(
        name=name,
        hostname=hostname,
        version=credential.get("version", "2c"),
        port=args.port,
        timeout=args.timeout,
        retries=args.retries,
        source="Netdata discovery config: {} ({})".format(scope.source, scope.subnet),
        community=credential.get("community"),
        username=credential.get("username"),
        level=credential.get("security_level"),
        auth_proto=credential.get("auth_protocol"),
        auth_key=credential.get("auth_password"),
        priv_proto=credential.get("priv_protocol"),
        priv_key=credential.get("priv_password"),
        context_name=credential.get("context_name", ""),
    )


def node_identity(node):
    return (
        node.hostname,
        node.version,
        node.port,
        node.timeout,
        node.retries,
        node.community,
        node.username,
        node.level,
        node.auth_proto,
        node.auth_key,
        node.priv_proto,
        node.priv_key,
        node.context_name,
    )


def add_selected_node(selected, selected_by_name, node):
    existing = selected_by_name.get(node.name)
    if existing is not None:
        if node_identity(existing) != node_identity(node):
            raise CollectionError(
                "node name {!r} selects conflicting addresses or SNMP settings; "
                "use a unique NAME for each target".format(node.name)
            )
        return
    selected_by_name[node.name] = node
    selected.append(node)


def configured_node_for_selector(selector, by_name, by_hostname):
    if "=" in selector:
        return None
    if selector in by_name:
        return by_name[selector]
    matches = by_hostname.get(selector, [])
    if len(matches) > 1:
        names = ", ".join(sorted(node.name for node in matches))
        raise CollectionError(
            "configured address {!r} belongs to multiple jobs ({}); select a job name".format(
                selector, names
            )
        )
    return matches[0] if matches else None


def discovery_node_for_selector(name, hostname, selector, discovery_scopes, args):
    if "=" in selector:
        return None
    try:
        address = ipaddress.ip_address(hostname)
    except ValueError:
        return None
    matches = [scope for scope in discovery_scopes if scope.contains(address)]
    if len(matches) > 1:
        raise CollectionError(
            "address {!r} matches multiple discovery credential scopes; use "
            "--node NAME=ADDRESS to provide manual settings".format(hostname)
        )
    return node_from_discovery(name, hostname, matches[0], args) if matches else None


def manual_node(name, hostname, args):
    return Node(
        name=name,
        hostname=hostname,
        version=args.snmp_version,
        port=args.port,
        timeout=args.timeout,
        retries=args.retries,
        username=args.v3_user,
        level=args.v3_level,
        auth_proto=args.v3_auth_proto,
        priv_proto=args.v3_priv_proto,
        context_name=args.v3_context,
    )


def select_nodes(configured, discovery_scopes, selectors, all_configured, args):
    selected = []
    selected_by_name = {}

    if all_configured:
        if not configured:
            raise CollectionError(
                "--all-configured requires static SNMP jobs. Discovery networks are never scanned; "
                "select exact addresses with --node"
            )
        for node in configured:
            add_selected_node(selected, selected_by_name, node)

    by_name = {node.name: node for node in configured}
    by_hostname = {}
    for node in configured:
        by_hostname.setdefault(node.hostname, []).append(node)

    for selector in selectors:
        node = configured_node_for_selector(selector, by_name, by_hostname)
        if node is not None:
            add_selected_node(selected, selected_by_name, node)
            continue
        name, hostname = parse_manual_selector(selector)
        node = discovery_node_for_selector(name, hostname, selector, discovery_scopes, args)
        add_selected_node(
            selected,
            selected_by_name,
            node if node is not None else manual_node(name, hostname, args),
        )

    if not selected:
        raise CollectionError("select at least one node with --node, or use --all-configured")
    return selected


_NETDATA_ENV_REFERENCE = re.compile(r"\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}")
_ACTIVE_REFERENCE = re.compile(r"\$\{[^{}:]*:[^{}]*\}")


def resolve_config_value(value, prompt, secret=False, default=None):
    value = scalar(value)
    if value is not None:
        def replace_netdata_env(match):
            value = os.environ.get(match.group(1))
            return match.group(0) if value is None else value.strip()

        value = _NETDATA_ENV_REFERENCE.sub(replace_netdata_env, value)
        # Netdata supports additional ${...} resolvers that a standalone tool
        # cannot reproduce safely. Prompt instead of using an unresolved token.
        if not _ACTIVE_REFERENCE.search(value):
            return value
    if default is not None and value is None:
        return default
    try:
        answer = getpass.getpass(prompt + ": ") if secret else input(prompt + ": ")
    except EOFError:
        raise CollectionError(
            "{} is not available from configuration or the environment, and input is not interactive".format(
                prompt
            )
        )
    if not answer:
        raise CollectionError("{} is required".format(prompt))
    return answer


def prepare_nodes(nodes):
    for node in nodes:
        node.hostname = resolve_config_value(node.hostname, "Address for {}".format(node.name))
        validate_hostname(node.hostname)
        if node.version in ("1", "2c"):
            node.community = resolve_config_value(
                node.community, "SNMP community for {}".format(node.name), secret=True
            )
            validate_secret(node.community, "SNMP community")
            continue

        node.username = resolve_config_value(node.username, "SNMPv3 username for {}".format(node.name))
        validate_secret(node.username, "SNMPv3 username")
        if node.level in ("authNoPriv", "authPriv"):
            node.auth_key = resolve_config_value(
                node.auth_key, "SNMPv3 authentication passphrase for {}".format(node.name), secret=True
            )
            validate_secret(node.auth_key, "SNMPv3 authentication passphrase")
        if node.level == "authPriv":
            node.priv_key = resolve_config_value(
                node.priv_key, "SNMPv3 privacy passphrase for {}".format(node.name), secret=True
            )
            validate_secret(node.priv_key, "SNMPv3 privacy passphrase")
        if node.context_name:
            node.context_name = resolve_config_value(
                node.context_name, "SNMPv3 context for {}".format(node.name)
            )
            validate_secret(node.context_name, "SNMPv3 context")


def validate_secret(value, label):
    if value is None or value == "":
        raise CollectionError("{} must not be empty".format(label))
    if any(character in value for character in "\x00\r\n"):
        raise CollectionError("{} contains an unsupported control character".format(label))


def validate_hostname(hostname):
    if any(character in hostname for character in "\x00\r\n\t "):
        raise CollectionError("SNMP address {!r} contains whitespace or a control character".format(hostname))
    try:
        ipaddress.ip_address(hostname)
        return
    except ValueError:
        pass
    if len(hostname) > 253 or not re.match(r"^[A-Za-z0-9][A-Za-z0-9._-]*$", hostname):
        raise CollectionError("SNMP address {!r} is not a valid IP address or hostname".format(hostname))


def find_snmpwalk(paths=SNMPWALK_PATHS):
    for path in paths:
        if not os.path.isfile(path) or not os.access(path, os.X_OK):
            continue
        info = os.stat(path)
        if info.st_mode & (stat.S_IWGRP | stat.S_IWOTH):
            continue
        if os.geteuid() == 0 and info.st_uid != 0:
            continue
        return path
    raise CollectionError(
        "snmpwalk was not found at {}. Install the Net-SNMP command-line tools and try again".format(
            " or ".join(paths)
        )
    )


def format_plan(nodes):
    protocols = ", ".join(name for name, _oids in PROBE_GROUPS)
    lines = [
        "SNMP data collection plan",
        "=========================",
        "No SNMP requests have been sent.",
        "",
    ]
    for index, node in enumerate(nodes, 1):
        lines.extend(
            (
                "{}. {}".format(index, node.name),
                "   Address: {}:{}".format(node.hostname, node.port),
                "   SNMP version: {}".format(node.version),
                "   Source: {}".format(node.source),
                "   Protocol data: {}".format(protocols),
                "",
            )
        )
    return "\n".join(lines).rstrip() + "\n"


def approve_plan(assume_yes):
    if assume_yes:
        print("Approval: accepted by --yes")
        return True
    try:
        answer = input("Proceed with this collection? [y/N]: ").strip().lower()
    except EOFError:
        return False
    return answer in ("y", "yes")


def net_snmp_quote(value):
    validate_secret(value, "SNMP configuration value")
    return '"{}"'.format(value.replace("\\", "\\\\").replace('"', '\\"'))


def write_private_snmp_config(directory, node):
    lines = ["dontLoadHostConfig true", "defVersion {}".format(node.version)]
    if node.version in ("1", "2c"):
        lines.append("defCommunity {}".format(net_snmp_quote(node.community)))
    else:
        lines.append("defSecurityName {}".format(net_snmp_quote(node.username)))
        lines.append("defSecurityLevel {}".format(node.level))
        if node.level in ("authNoPriv", "authPriv"):
            lines.append("defAuthType {}".format(node.auth_proto))
            lines.append("defAuthPassphrase {}".format(net_snmp_quote(node.auth_key)))
        if node.level == "authPriv":
            lines.append("defPrivType {}".format(node.priv_proto))
            lines.append("defPrivPassphrase {}".format(net_snmp_quote(node.priv_key)))
        if node.context_name:
            lines.append("defContext {}".format(net_snmp_quote(node.context_name)))
    path = os.path.join(directory, SNMP_CONFIG_FILENAME)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        payload = ("\n".join(lines) + "\n").encode("utf-8")
        offset = 0
        while offset < len(payload):
            offset += os.write(descriptor, payload[offset:])
    finally:
        os.close(descriptor)
    return path


def net_snmp_target(hostname, port):
    try:
        address = ipaddress.ip_address(hostname)
    except ValueError:
        address = None
    if address is not None and address.version == 6:
        return "udp6:[{}]:{}".format(hostname, port)
    return "udp:{}:{}".format(hostname, port)


def terminate_process_group(process):
    with defer_termination_signals():
        if process.poll() is not None:
            return
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            return
        try:
            process.wait(timeout=1)
            return
        except subprocess.TimeoutExpired:
            pass
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()


def handle_termination_signal(signum, _frame):
    global _PENDING_TERMINATION_SIGNAL
    global _TERMINATION_IN_PROGRESS
    if _TERMINATION_IN_PROGRESS:
        return
    if _TERMINATION_DEFER_DEPTH:
        if _PENDING_TERMINATION_SIGNAL is None:
            _PENDING_TERMINATION_SIGNAL = signum
        return
    _TERMINATION_IN_PROGRESS = True
    raise CollectionInterrupted(signum)


@contextmanager
def defer_termination_signals():
    global _PENDING_TERMINATION_SIGNAL
    global _TERMINATION_DEFER_DEPTH
    global _TERMINATION_IN_PROGRESS
    _TERMINATION_DEFER_DEPTH += 1
    try:
        yield
    finally:
        _TERMINATION_DEFER_DEPTH -= 1
        if _TERMINATION_DEFER_DEPTH == 0 and _PENDING_TERMINATION_SIGNAL is not None:
            signum = _PENDING_TERMINATION_SIGNAL
            _PENDING_TERMINATION_SIGNAL = None
            _TERMINATION_IN_PROGRESS = True
            raise CollectionInterrupted(signum)


def install_termination_handlers():
    global _PENDING_TERMINATION_SIGNAL
    global _TERMINATION_DEFER_DEPTH
    global _TERMINATION_IN_PROGRESS
    _PENDING_TERMINATION_SIGNAL = None
    _TERMINATION_DEFER_DEPTH = 0
    _TERMINATION_IN_PROGRESS = False
    previous = {}
    for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
        previous[signum] = signal.signal(signum, handle_termination_signal)
    return previous


def restore_signal_handlers(previous):
    global _PENDING_TERMINATION_SIGNAL
    global _TERMINATION_DEFER_DEPTH
    global _TERMINATION_IN_PROGRESS
    for signum, handler in previous.items():
        signal.signal(signum, handler)
    _PENDING_TERMINATION_SIGNAL = None
    _TERMINATION_DEFER_DEPTH = 0
    _TERMINATION_IN_PROGRESS = False


def command_output_state(process, descriptor, deadline):
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        return "timeout"
    readable, _writable, _exceptional = select.select(
        [descriptor], [], [], min(0.2, remaining)
    )
    if readable:
        return "read"
    if process.poll() is None:
        return "wait"
    readable, _writable, _exceptional = select.select([descriptor], [], [], 0)
    return "read" if readable else "done"


def read_nonblocking(descriptor):
    try:
        return os.read(descriptor, 65536)
    except OSError as error:
        if error.errno in (errno.EAGAIN, errno.EWOULDBLOCK):
            return None
        raise


def write_bounded_chunk(output, chunk, written, limit):
    kept = chunk[:max(0, limit - written)]
    if kept:
        output.write(kept)
    return written + len(kept), len(kept) != len(chunk)


def drain_bounded_output(process, descriptor, output, deadline, limit):
    written = 0
    truncated = False
    timed_out = False
    while True:
        state = command_output_state(process, descriptor, deadline)
        if state == "timeout":
            terminate_process_group(process)
            timed_out = True
            break
        if state == "wait":
            continue
        if state == "done":
            break
        chunk = read_nonblocking(descriptor)
        if chunk is None:
            continue
        if not chunk:
            break
        written, chunk_truncated = write_bounded_chunk(output, chunk, written, limit)
        truncated = truncated or chunk_truncated
    return timed_out, truncated, written


def finish_process(process, deadline):
    if process.poll() is not None:
        return False
    try:
        process.wait(timeout=max(0, deadline - time.monotonic()))
        return False
    except subprocess.TimeoutExpired:
        terminate_process_group(process)
        return True


def run_bounded(command, environment, output, timeout, limit=MAX_COMMAND_OUTPUT):
    process = None
    try:
        with defer_termination_signals():
            # The executable and argument list are validated; no shell is used.
            process = subprocess.Popen(  # nosec B603
                command,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                env=environment,
                start_new_session=True,
            )
        descriptor = process.stdout.fileno()
        flags = fcntl.fcntl(descriptor, fcntl.F_GETFL)
        fcntl.fcntl(descriptor, fcntl.F_SETFL, flags | os.O_NONBLOCK)
        deadline = time.monotonic() + timeout
        timed_out, truncated, written = drain_bounded_output(
            process, descriptor, output, deadline, limit
        )
        timed_out = finish_process(process, deadline) or timed_out
    except BaseException:
        with defer_termination_signals():
            if process is not None:
                terminate_process_group(process)
        raise
    finally:
        with defer_termination_signals():
            if process is not None and process.stdout is not None:
                process.stdout.close()
    return CommandResult(process.returncode, timed_out, truncated, written)


def safe_name(value):
    cleaned = re.sub(r"[^A-Za-z0-9._-]+", "_", value).strip("._-")
    return cleaned[:80] or "node"


def write_text(path, text):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        payload = text.encode("utf-8")
        offset = 0
        while offset < len(payload):
            offset += os.write(descriptor, payload[offset:])
    finally:
        os.close(descriptor)


def collect_probe(snmpwalk, node, oids, config_dir, output_path, command_timeout):
    environment = os.environ.copy()
    environment["SNMPCONFPATH"] = config_dir
    successes = 0
    failures = 0
    truncated = False
    details = []
    with open(output_path, "wb") as output:
        for oid in oids:
            command = [
                snmpwalk,
                "-On",
                "-t",
                str(node.timeout),
                "-r",
                str(node.retries),
                net_snmp_target(node.hostname, node.port),
                oid,
            ]
            visible = "SNMPCONFPATH={} {}".format(
                shlex.quote(config_dir), " ".join(shlex.quote(item) for item in command)
            )
            print("    $ {}".format(visible))
            output.write(("\n### OID {}\n$ {}\n".format(oid, visible)).encode("utf-8"))
            result = run_bounded(command, environment, output, command_timeout)
            output.write(b"\n")
            if result.timed_out:
                failures += 1
                details.append("{} timed out".format(oid))
            elif result.returncode != 0:
                failures += 1
                details.append("{} exited {}".format(oid, result.returncode))
            else:
                successes += 1
            if result.truncated:
                truncated = True
                details.append("{} output truncated".format(oid))

    if failures == 0 and not truncated:
        status_value = "success"
    elif successes > 0:
        status_value = "partial"
    else:
        status_value = "failed"
    return status_value, "; ".join(details) or "collected"


def build_manifest(root):
    lines = []
    for directory, _subdirectories, filenames in os.walk(root):
        for filename in sorted(filenames):
            path = os.path.join(directory, filename)
            relative = os.path.relpath(path, root)
            digest = hashlib.sha256()
            with open(path, "rb") as handle:
                while True:
                    chunk = handle.read(1024 * 1024)
                    if not chunk:
                        break
                    digest.update(chunk)
            lines.append("{}  {}".format(digest.hexdigest(), relative))
    write_text(os.path.join(root, "manifest.sha256"), "\n".join(lines) + "\n")


def create_archive(root, output_dir):
    if not os.path.isdir(output_dir):
        raise CollectionError("output directory does not exist: {}".format(output_dir))
    timestamp = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    archive_path = os.path.abspath(
        os.path.join(output_dir, "netdata-snmp-data-{}.tar.gz".format(timestamp))
    )
    suffix = 1
    while os.path.exists(archive_path):
        archive_path = os.path.abspath(
            os.path.join(output_dir, "netdata-snmp-data-{}-{}.tar.gz".format(timestamp, suffix))
        )
        suffix += 1
    descriptor = -1
    archive_created = False
    try:
        with defer_termination_signals():
            descriptor = os.open(archive_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            archive_created = True
        with os.fdopen(descriptor, "wb") as archive_file:
            descriptor = -1
            with tarfile.open(fileobj=archive_file, mode="w:gz") as archive:
                for name in sorted(os.listdir(root)):
                    archive.add(os.path.join(root, name), arcname=name, recursive=True)
    except BaseException:
        with defer_termination_signals():
            if descriptor >= 0:
                os.close(descriptor)
            if archive_created:
                try:
                    os.unlink(archive_path)
                except OSError:
                    pass
        raise
    if os.geteuid() == 0 and os.environ.get("SUDO_UID") and os.environ.get("SUDO_GID"):
        try:
            os.chown(archive_path, int(os.environ["SUDO_UID"]), int(os.environ["SUDO_GID"]))
        except (OSError, ValueError) as error:
            print("WARNING: could not return archive ownership to the sudo user: {}".format(error), file=sys.stderr)
    return archive_path


def collect(nodes, plan, snmpwalk, output_dir, command_timeout):
    private_root = None
    try:
        with defer_termination_signals():
            private_root = tempfile.mkdtemp(prefix="netdata-snmp-", dir=output_dir)
        os.chmod(private_root, 0o700)
        data_root = os.path.join(private_root, "archive")
        secrets_root = os.path.join(private_root, "credentials")
        os.mkdir(data_root, 0o700)
        os.mkdir(secrets_root, 0o700)
        os.mkdir(os.path.join(data_root, "devices"), 0o700)
        rows = []
        write_text(os.path.join(data_root, "plan.txt"), plan)
        write_text(
            os.path.join(data_root, "README.txt"),
            "This archive contains raw SNMP responses collected for Netdata troubleshooting.\n"
            "SNMP credentials are not included. Raw responses can contain sensitive network,\n"
            "inventory, addressing, and device configuration information. Share the archive\n"
            "only through the restricted support ticket requested by Netdata.\n",
        )
        total = len(nodes) * len(PROBE_GROUPS)
        current = 0
        for node_index, node in enumerate(nodes, 1):
            node_dir = os.path.join(
                data_root,
                "devices",
                "{:03d}-{}".format(node_index, safe_name(node.name)),
            )
            credential_dir = os.path.join(secrets_root, "{:03d}".format(node_index))
            os.mkdir(node_dir, 0o700)
            os.mkdir(credential_dir, 0o700)
            write_private_snmp_config(credential_dir, node)
            for group_name, oids in PROBE_GROUPS:
                current += 1
                print("[{}/{}] {}: {}".format(current, total, node.name, group_name))
                output_path = os.path.join(node_dir, group_name + ".txt")
                try:
                    status_value, detail = collect_probe(
                        snmpwalk,
                        node,
                        oids,
                        credential_dir,
                        output_path,
                        command_timeout,
                    )
                except Exception as error:
                    status_value = "failed"
                    detail = "collector error: {}".format(error)
                    write_text(output_path, detail + "\n")
                rows.append((node.name, node.hostname, group_name, status_value, detail))
                print("    Result: {}{}".format(status_value, " ({})".format(detail) if detail != "collected" else ""))

        header = "node\taddress\tprotocol\tstatus\tdetails\n"
        status_text = header + "".join(
            "{}\t{}\t{}\t{}\t{}\n".format(
                clean_tsv(name), clean_tsv(address), clean_tsv(protocol), status_value, clean_tsv(detail)
            )
            for name, address, protocol, status_value, detail in rows
        )
        write_text(os.path.join(data_root, "collection-status.tsv"), status_text)
        summary = summarize(rows)
        write_text(os.path.join(data_root, "summary.txt"), summary)
        build_manifest(data_root)
        archive_path = create_archive(data_root, output_dir)
        return rows, summary, archive_path
    finally:
        with defer_termination_signals():
            if private_root is not None:
                shutil.rmtree(private_root, ignore_errors=True)


def clean_tsv(value):
    return str(value).replace("\t", " ").replace("\r", " ").replace("\n", " ")


def summarize(rows):
    counts = {"success": 0, "partial": 0, "failed": 0}
    for _name, _address, _protocol, status_value, _detail in rows:
        counts[status_value] += 1
    return (
        "SNMP collection summary\n"
        "=======================\n"
        "Successful protocol collections: {success}\n"
        "Partial protocol collections: {partial}\n"
        "Failed protocol collections: {failed}\n".format(**counts)
    )


def build_parser():
    parser = argparse.ArgumentParser(
        description="Collect raw SNMP data for Netdata troubleshooting.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Examples:\n"
            "  %(prog)s --node 192.0.2.10\n"
            "  %(prog)s --node core-switch=192.0.2.10 --snmp-version 3 --v3-user operator\n"
            "  %(prog)s --node core-switch\n"
            "  %(prog)s --all-configured\n"
            "  %(prog)s --netdata-snmp-config /private/snmp.conf --list-configured\n\n"
            "With no explicit configuration path, the standard SNMP jobs and discovery files under\n"
            "/etc/netdata are used first, then those under /opt/netdata/etc/netdata. A configured\n"
            "job name or address reuses that job. An exact IP inside one discovery credential scope\n"
            "reuses that credential without scanning. Other --node values are manual targets."
        ),
    )
    parser.add_argument("--version", action="version", version="%(prog)s " + VERSION)
    parser.add_argument(
        "--node",
        action="append",
        default=[],
        metavar="[NAME=]ADDRESS_OR_JOB",
        help="node to collect; repeat for multiple nodes",
    )
    parser.add_argument(
        "--all-configured",
        action="store_true",
        help="collect every job from the selected Netdata SNMP config",
    )
    parser.add_argument(
        "--list-configured",
        action="store_true",
        help="list jobs and discovery credential scopes from Netdata config and exit",
    )
    parser.add_argument(
        "--netdata-snmp-config",
        metavar="FILE",
        help="use this Netdata go.d SNMP configuration file",
    )
    parser.add_argument(
        "--snmp-version",
        default="2c",
        choices=("1", "2", "2c", "3"),
        help="SNMP version for manual nodes (default: 2c)",
    )
    parser.add_argument(
        "--port", type=int, default=161, help="SNMP port for manual nodes (default: 161)"
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=5,
        help="SNMP request timeout for manual nodes (default: 5 seconds)",
    )
    parser.add_argument(
        "--retries", type=int, default=1, help="SNMP retries for manual nodes (default: 1)"
    )
    parser.add_argument(
        "--v3-user", help="SNMPv3 username for manual nodes; prompted when omitted"
    )
    parser.add_argument(
        "--v3-level",
        default="authPriv",
        choices=("none", "authNoPriv", "authPriv"),
        help="SNMPv3 security level for manual nodes (default: authPriv)",
    )
    parser.add_argument(
        "--v3-auth-proto",
        default="sha512",
        choices=("md5", "sha", "sha224", "sha256", "sha384", "sha512"),
        help="SNMPv3 authentication protocol for manual nodes (default: sha512)",
    )
    parser.add_argument(
        "--v3-priv-proto",
        default="aes",
        choices=("des", "aes", "aes192", "aes256", "aes192c", "aes256c"),
        help="SNMPv3 privacy protocol for manual nodes (default: aes)",
    )
    parser.add_argument(
        "--v3-context",
        default="",
        help="SNMPv3 context name for manual nodes (default: empty)",
    )
    parser.add_argument(
        "--command-timeout",
        type=int,
        default=120,
        help="maximum time for each SNMP walk (default: 120 seconds)",
    )
    parser.add_argument(
        "--output-dir",
        default=".",
        metavar="DIRECTORY",
        help="directory for the final tar.gz (default: current directory)",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="accept the printed plan without an interactive confirmation",
    )
    return parser


def list_configured_jobs(nodes, scopes, paths):
    print("Netdata SNMP configuration from {}:".format(", ".join(paths)))
    for node in nodes:
        print("  job  {}  {}:{}  SNMP {}".format(node.name, node.hostname, node.port, node.version))
    for scope in scopes:
        print(
            "  discovery scope  {}  SNMP {}  (select an exact address with --node)".format(
                scope.subnet, normalize_version(scope.credential.get("version", "2c"))
            )
        )
    if not nodes and not scopes:
        print("  (no enabled jobs or discovery credential scopes)")


def run(args):
    try:
        args.port = positive_int(args.port, "--port")
        args.timeout = positive_int(args.timeout, "--timeout")
        args.retries = nonnegative_int(args.retries, "--retries")
        args.command_timeout = positive_int(args.command_timeout, "--command-timeout")
        manual_only = (
            bool(args.node)
            and all("=" in selector for selector in args.node)
            and args.netdata_snmp_config is None
            and not args.list_configured
            and not args.all_configured
        )
        if manual_only:
            config_paths = []
            configured, discovery_scopes = [], []
        else:
            config_paths = select_netdata_configs(args.netdata_snmp_config)
            configured, discovery_scopes = read_netdata_configuration(
                config_paths, skip_empty=args.netdata_snmp_config is None
            )
        if args.list_configured:
            if not config_paths:
                raise CollectionError(
                    "no Netdata SNMP config was found; use --netdata-snmp-config FILE"
                )
            list_configured_jobs(configured, discovery_scopes, config_paths)
            return 0
        nodes = select_nodes(
            configured, discovery_scopes, args.node, args.all_configured, args
        )
        prepare_nodes(nodes)
        snmpwalk = find_snmpwalk()
        output_dir = os.path.abspath(args.output_dir)
        if not os.path.isdir(output_dir):
            raise CollectionError("output directory does not exist: {}".format(output_dir))
        plan = format_plan(nodes)
        print(plan)
        if not approve_plan(args.yes):
            print("Collection cancelled. No SNMP requests were sent.")
            return 0
        rows, summary, archive_path = collect(
            nodes, plan, snmpwalk, output_dir, args.command_timeout
        )
        print("\n" + summary)
        print("Archive: {}".format(archive_path))
        print("Next step: attach this tar.gz to a restricted Freshdesk support ticket.")
        return 0 if all(row[3] == "success" for row in rows) else 2
    except CollectionError as error:
        print("ERROR: {}".format(error), file=sys.stderr)
        return 1
    except CollectionInterrupted as interruption:
        signal_name = signal.Signals(interruption.signum).name
        print(
            "\nCollection interrupted by {}. No incomplete archive was kept.".format(signal_name),
            file=sys.stderr,
        )
        return 128 + interruption.signum


def main(argv=None):
    direct_cli = argv is None
    argv = sys.argv[1:] if argv is None else argv
    parser = build_parser()
    if not argv:
        parser.print_help()
        return 0
    args = parser.parse_args(argv)
    previous_handlers = install_termination_handlers()
    try:
        result = run(args)
    except BaseException:
        restore_signal_handlers(previous_handlers)
        raise
    if not (direct_cli and _TERMINATION_IN_PROGRESS):
        restore_signal_handlers(previous_handlers)
    return result


if __name__ == "__main__":
    sys.exit(main())
