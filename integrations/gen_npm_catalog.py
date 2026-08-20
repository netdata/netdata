#!/usr/bin/env python3
"""
Generate the Network Performance Monitoring (NPM) integration catalog metadata
from the SNMP device profiles.

Reads the SNMP device profiles under
    src/go/plugin/go.d/config/go.d/snmp.profiles/default/
and the SNMP trap-profile catalogue at
    src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json
and emits
    src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml

with one entry per device vendor (Device Metrics), per BGP-capable vendor plus a
generic BGP4-MIB entry (BGP Monitoring), per licensing-capable vendor (Licensing
Monitoring), per topology discovery method/producer (Topologies), the syslog
entry (Syslog), one entry per trap-profile vendor (SNMP Traps), and the SNMP
trap enrichment methods.

The emitted file is consumed by gen_integrations.py as integration_type=device
and renders under the Network Performance Monitoring category on Learn, the
website, and cloud-frontend. Capability (BGP / licensing) is detected by
resolving each profile's transitive `extends:` chain.

DO NOT hand-edit the generated metadata.yaml; edit this generator or the
profiles instead. Run: `python3 integrations/gen_npm_catalog.py`.
"""

import json
import re
from pathlib import Path

from ruamel.yaml import YAML

REPO_PATH = Path(__file__).resolve().parent.parent
PROFILES_DIR = REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'config' / 'go.d' / 'snmp.profiles' / 'default'
TRAP_CATALOGUE = (REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'config' / 'go.d'
                  / 'snmp.trap-profiles' / 'catalogue.json')
TRAP_PROFILES_DIR = TRAP_CATALOGUE.parent / 'default'
OUTPUT = REPO_PATH / 'src' / 'go' / 'plugin' / 'go.d' / 'collector' / 'snmp' / 'npm-catalog' / 'metadata.yaml'

# A per-MIB trap breakdown longer than this is folded behind <details> to keep
# the page scannable; ~94% of trap vendors fall at or under it and render inline.
TRAP_FOLD_MIBS = 30

# Canonical SNMP-trap taxonomy, mirroring snmptrapprofilegen (validCategories,
# severityPriority). Display order: severities by syslog priority (most severe
# first), categories in the Go declaration order. These are the exact slugs the
# Logs UI filters on (TRAP_SEVERITY / TRAP_CATEGORY), so they are shown verbatim.
TRAP_SEVERITY_ORDER = ['emerg', 'alert', 'crit', 'err', 'warning', 'notice', 'info', 'debug']
TRAP_CATEGORY_ORDER = ['state_change', 'config_change', 'security', 'auth',
                       'license', 'mobility', 'diagnostic', 'unknown']

CAT_DEVICE = 'network-performance-monitoring.device-metrics'
CAT_BGP = 'network-performance-monitoring.bgp'
CAT_LICENSING = 'network-performance-monitoring.licensing'
CAT_TOPOLOGY = 'network-performance-monitoring.topologies'
CAT_SYSLOG = 'network-performance-monitoring.syslog'
CAT_TRAPS = 'network-performance-monitoring.traps'

# Corporate suffixes / acronyms that should not be naively title-cased when
# humanizing a hyphenated IANA enterprise slug into a vendor display name.
VENDOR_SUFFIX_FIXES = {
    'inc': 'Inc', 'ltd': 'Ltd', 'llc': 'LLC', 'gmbh': 'GmbH', 'co': 'Co',
    'corp': 'Corp', 'sa': 'S.A.', 'ag': 'AG', 'bv': 'B.V.', 'plc': 'PLC',
    'nv': 'N.V.', 'oy': 'Oy', 'as': 'AS', 'spa': 'S.p.A.', 'srl': 'S.r.l.',
    'kg': 'KG', 'ab': 'AB', 'usa': 'USA', 'uk': 'UK', 'gmbh.': 'GmbH',
}

BGP_RE = re.compile(r'bgp', re.IGNORECASE)
LICENSE_RE = re.compile(r'licens', re.IGNORECASE)

# Vendors whose logo is published at netdata.cloud/img/<icon>. Anything not
# listed falls back to the generic SNMP icon. Keep this conservative: a wrong
# icon name renders a broken image, the SNMP fallback never does. Every entry
# is verified to exist on the CDN, and .png vs .svg matters (only some logos
# are vector). Keys are a short device-vendor name ('juniper') or a normalized
# IANA enterprise slug ('hewlettpackard'); icon_for() also matches the longest
# brand prefix so trap slugs like 'junipernetworksinc' resolve to the brand.
VENDOR_ICONS = {
    'cisco': 'cisco.svg',
    'juniper': 'juniper.png',
    'huawei': 'huawei.svg',
    'fortinet': 'fortinet.svg',
    'mikrotik': 'mikrotik.png',
    'paloalto': 'paloalto.png',
    'paloaltonetworks': 'paloalto.png',
    'netapp': 'netapp.svg',
    'vmware': 'vmware.svg',
    'nvidia': 'nvidia.svg',
    'dell': 'dell.svg',
    'hp': 'hp.svg',
    'hewlettpackard': 'hp.svg',
    'hpe': 'hpe.png',
    'hewlettpackardenterprise': 'hpe.png',
    'ibm': 'ibm.svg',
    # 'arista': pending a usable logo on the CDN (only white-on-transparent
    # variants exist today); falls back to the SNMP icon until netdata/website
    # ships a visible arista.png.
}
FALLBACK_ICON = 'SNMP.png'

# Most catalog entries are served by go.d.plugin; the exceptions name their own
# plugin (network-viewer.plugin, otel.plugin, netdata).
PLUGIN_GO_D = 'go.d.plugin'

# Selectable topology-role profiles add capabilities to matched devices; they
# are not standalone device families and must not produce device catalog pages.
INTERNAL_TOPOLOGY_ROLE_PREFIX = 'topology-role-'

# Page descriptions that cannot be mechanically shortened without losing a
# complete, profile-specific statement. Keys are the rendered page names.
PAGE_DESCRIPTIONS = {
    'A10 Thunder': (
        'Monitor A10 Thunder ADC metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'APC PDU': (
        'Monitor APC PDU power distribution metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'APC UPS': (
        'Monitor APC UPS power and battery metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'BGP Peering Topology': (
        'Map BGP peer relationships between routers, including remote autonomous system '
        'numbers and session state from standard and vendor MIBs.'
    ),
    'Cisco ICM': (
        'Monitor Cisco ICM device metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'Cisco SB': (
        'Monitor Cisco SB device metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'Cisco UCS': (
        'Monitor Cisco UCS server metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'Eaton Epdu': (
        'Monitor Eaton ePDU power distribution metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'Eaton UPS': (
        'Monitor Eaton UPS power and battery metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'Exagrid': (
        'Monitor ExaGrid storage metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'HP ILO': (
        'Monitor HP iLO server hardware metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'HP Ilo4': (
        'Monitor HP iLO 4 server hardware metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'HPE MSA': (
        'Monitor HPE MSA storage metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'IDRAC': (
        'Monitor Dell iDRAC server hardware metrics over SNMP with automatic profile '
        'recognition and no manual OID configuration.'
    ),
    'Isilon': (
        'Monitor Dell Isilon storage metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'Live Network Connections': (
        'Map live application dependencies from host socket tables, including local '
        'processes, remote endpoints, and Linux workload ownership.'
    ),
    'Peplink': (
        'Monitor Peplink router metrics over SNMP with automatic profile recognition '
        'and no manual OID configuration.'
    ),
    'SNMP Trap Reverse DNS Enrichment': (
        'Add reverse DNS hostnames to SNMP traps through cached PTR lookups of their '
        'source addresses.'
    ),
    'SNMP Trap Node Attribution': (
        'Attribute each SNMP trap to an unambiguous Netdata node when possible, '
        'otherwise retaining it under a bounded source label.'
    ),
    'vSphere Topology': (
        'Map VMware vSphere clusters, hosts, virtual machines, datastores, networks, '
        'and port groups with placement and utilization overlays.'
    ),
}


def load_profiles():
    """Return {filename: {'extends': [...], 'data': dict, 'text': str}} for every profile/module."""
    yaml = YAML(typ='safe')
    profiles = {}
    for path in sorted(PROFILES_DIR.glob('*.yaml')):
        text = path.read_text(encoding='utf-8')
        try:
            data = yaml.load(text) or {}
        except Exception:
            data = {}
        profiles[path.name] = {
            'extends': list(data.get('extends', []) or []),
            'data': data,
            'text': text,
        }
    return profiles


def is_device_catalog_profile(name):
    """Return whether a profile represents a public device integration."""
    return not name.startswith('_') and not name.startswith(INTERNAL_TOPOLOGY_ROLE_PREFIX)


def resolve_extends(name, profiles, seen=None):
    """Return the set of all module filenames reachable from `name` via transitive extends."""
    if seen is None:
        seen = set()
    for ext in profiles.get(name, {}).get('extends', []):
        if ext in seen:
            continue
        seen.add(ext)
        resolve_extends(ext, profiles, seen)
    return seen


def device_field(data, field):
    return (((data.get('metadata') or {}).get('device') or {}).get('fields') or {}).get(field, {}).get('value')


def inherited_field(reachable, profiles, field):
    """First value of `field` found along the (sorted, deterministic) extends chain."""
    for m in sorted(reachable):
        if m in profiles:
            v = device_field(profiles[m]['data'], field)
            if v:
                return v
    return None


def collect_vendors(profiles):
    """Group device profiles (non-underscore files) by vendor with capability flags.

    Vendor/type are read from the profile itself OR inherited through its
    `extends:` chain — many concrete model profiles (e.g. Arista, Huawei) set
    these only on their vendor base.
    """
    vendors = {}
    for name, p in profiles.items():
        if not is_device_catalog_profile(name):
            continue
        reachable = resolve_extends(name, profiles)
        vendor = device_field(p['data'], 'vendor') or inherited_field(reachable, profiles, 'vendor')
        if not vendor:
            continue
        dev_type = device_field(p['data'], 'type') or inherited_field(reachable, profiles, 'type')

        # Capability by extends-chain filename match (a vendor base/mixin like
        # `_cisco-bgp4-mib.yaml` or `_checkpoint-licensing.yaml`), NOT by any
        # "licens"/"bgp" word in the text — a "licensed APs" metric is not
        # licensing telemetry.
        has_bgp = any(BGP_RE.search(m) for m in reachable)
        has_lic = any(LICENSE_RE.search(m) for m in reachable)

        key = vendor.lower()
        v = vendors.setdefault(key, {'display': vendor, 'types': set(), 'profiles': 0, 'bgp': False, 'lic': False})
        # Prefer the display variant with the most uppercase characters (HP over hp).
        if sum(c.isupper() for c in vendor) > sum(c.isupper() for c in v['display']):
            v['display'] = vendor
        if dev_type:
            v['types'].add(dev_type)
        v['profiles'] += 1
        v['bgp'] = v['bgp'] or has_bgp
        v['lic'] = v['lic'] or has_lic
    return vendors


def icon_for(vendor_key):
    """Resolve a brand icon from a vendor name or IANA enterprise slug.

    Device profiles pass a short vendor name ('juniper'); the trap catalogue
    passes the IANA slug ('juniper-networks-inc', 'ibm-eserver-x'). Match exact
    first, then the longest brand prefix. Prefix matching is restricted to keys
    >= 3 chars: that excludes only the 2-char 'hp' key (too ambiguous to prefix
    on; HP is still resolved by its exact entry and the 'hewlettpackard' alias),
    while distinctive 3-char brands like 'ibm'/'hpe' still brand their slugs."""
    k = re.sub(r'[^a-z0-9]', '', (vendor_key or '').lower())
    if not k:
        return FALLBACK_ICON
    if k in VENDOR_ICONS:
        return VENDOR_ICONS[k]
    for token in sorted((t for t in VENDOR_ICONS if len(t) >= 3), key=len, reverse=True):
        if k.startswith(token):
            return VENDOR_ICONS[token]
    return FALLBACK_ICON


def humanize_vendor(slug):
    """Turn an IANA-enterprise slug ('hewlett-packard', '3com') into a display
    name ('Hewlett Packard', '3Com'), fixing common corporate suffixes."""
    return ' '.join(VENDOR_SUFFIX_FIXES.get(w, w.title()) for w in slug.split('-'))


def _plural(n, word):
    return f'{n} {word}' if n == 1 else f'{n} {word}s'


def overview(metrics_description, method_description, auto_detection, platforms=(), multi_instance=True,
             limits='', performance_impact='', permissions=''):
    """`platforms` empty means every platform the Agent runs on; producers that are not
    built everywhere must list theirs. `multi_instance` must be False for singleton
    producers, otherwise the page claims side-by-side instances the collector rejects.

    An empty `limits` renders "does not impose any limits", so pass the real limit
    whenever the producer has one."""
    return {
        'data_collection': {'metrics_description': metrics_description, 'method_description': method_description},
        'supported_platforms': {'include': list(platforms), 'exclude': []},
        'multi_instance': multi_instance,
        'additional_permissions': {'description': permissions},
        'default_behavior': {
            'auto_detection': {'description': auto_detection},
            'limits': {'description': limits},
            'performance_impact': {'description': performance_impact},
        },
    }


def setup_block(config_file, options_description, prerequisites=(), options=(), examples=(), section_name=None,
                single_job=False):
    """Build a `setup` block. Most catalog entries come from the SNMP collector and
    share SETUP below, but the topology, syslog, and vSphere/Cato producers are not
    SNMP-based and must not inherit SNMP prerequisites or `go.d/snmp.conf`.

    An empty `config_file` renders "There is no configuration file." and an empty
    `prerequisites` renders "No action required." (integrations/templates/setup-generic.md).
    Pass `options` whenever the producer really does expose settings — an empty list
    renders as if it had none.

    `single_job` marks a collector that rejects user-defined jobs (go.d
    `InstancePolicySingle`). The generic go.d rendering otherwise tells the reader to
    add a job from the UI and shows a multi-job sample, neither of which works. This is
    NOT the same as `multi_instance: false`, which many collectors set while still
    accepting jobs."""
    block = {
        'prerequisites': {'list': [{'title': t, 'description': d} for t, d in prerequisites]},
        'configuration': {
            'file': {'name': config_file},
            'options': {
                'description': options_description,
                'folding': {'title': 'Config options', 'enabled': True},
                'list': list(options),
            },
            'examples': {'folding': {'title': 'Config', 'enabled': True}, 'list': list(examples)},
        },
    }
    if section_name:
        block['configuration']['file']['section_name'] = section_name
    if single_job:
        block['single_job'] = True
    return block


SETUP = setup_block(
    'go.d/snmp.conf',
    'Configure the SNMP collector with the device hostname and SNMP credentials. See the SNMP collector reference for '
    'all options.',
    [('SNMP access',
      'SNMP must be enabled on the device and reachable from the Netdata Agent acting as the site\'s SNMP hub.')],
)

# The topology collector has its own job file; the devices it walks come from the
# SNMP collector's jobs (src/go/plugin/go.d/config/go.d/snmp_topology.conf). It is a
# single-instance collector (InstancePolicySingle), and its schema exposes exactly the
# two options below (snmp_topology/config_schema.json).
SNMP_TOPOLOGY_SETUP = setup_block(
    'go.d/snmp_topology.conf',
    'Topology discovery needs no per-device configuration: it walks the devices already configured as SNMP collector '
    'jobs. The only settings are the two intervals below, and `update_every` has a minimum of 10.',
    [('SNMP devices configured',
      'The devices must already be collected over SNMP (`go.d/snmp.conf`), and SNMP access must be reachable from the '
      'Netdata Agent acting as the site\'s SNMP hub.')],
    options=[
        {'name': 'update_every', 'description': 'How often to check for new or stale devices, in seconds.',
         'default_value': 60, 'required': False},
        {'name': 'refresh_every', 'description': 'How often to refresh topology data for each device.',
         'default_value': '30m', 'required': False},
    ],
    single_job=True,
)

# network-viewer.plugin reads the host's own socket table: nothing to reach and no job to
# create, but it is not option-free. Mirror the collector's own metadata
# (src/collectors/network-viewer.plugin/metadata.yaml) rather than claiming "no configuration".
NETWORK_VIEWER_SETUP = setup_block(
    'netdata.conf',
    'The Function is always available and needs no setup. The only setting is the size of the per-PID APPS_LOOKUP '
    'cache, which holds the cgroup identity already resolved for each process; raising it does not make more '
    'processes resolvable, it only keeps more resolved ones cached.',
    [('Privileged access to the socket tables',
      'The plugin needs its normal privileged permissions to enumerate the sockets of every process. Standard '
      'installations grant these. A container needs the host network namespace and the host `/proc` to see anything '
      'beyond its own sockets, `SYS_ADMIN` to reach sibling containers\' connections, and `SYS_PTRACE` to attribute '
      'connections to processes — without `SYS_PTRACE` the connections are still listed but not tied to the '
      'processes that own them. On macOS, a non-privileged or TCC-restricted run silently omits protected '
      'processes.')],
    section_name='[plugin:network-viewer]',
    options=[
        {'name': 'apps lookup cache size',
         'description': 'Maximum number of per-PID APPS_LOOKUP cache entries kept by network-viewer.plugin.',
         'default_value': 8192, 'required': False},
    ],
    # No example: the shared template fences every example as YAML, which would
    # mislabel netdata.conf's INI syntax. The options table above is enough.
)

STREAMING_SETUP = setup_block(
    'stream.conf',
    'The streaming topology reflects whatever streaming is already configured. Use this file to change how this Agent '
    'streams to, or accepts streams from, other Agents.',
    [('Streaming configured',
      'At least two Netdata Agents connected through streaming. A standalone Agent renders a single node.')],
)

VSPHERE_SETUP = setup_block(
    'go.d/vsphere.conf',
    'Configure the vSphere collector with the vCenter URL and credentials. See the '
    '[vSphere collector](/src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md) page for '
    'all options, including `collect_network_topology`, which is off by default and is what adds the network '
    'and port-group actors to the map.',
    [('vCenter access',
      'A vCenter Server reachable from the Netdata Agent, and a read-only account with permission to browse the '
      'inventory.')],
    examples=[{
        'name': 'Minimal job with topology enabled',
        'folding': {'enabled': False},
        'description': 'The three required fields plus `collect_network_topology`, which is off by default and is what '
                       'adds the network and port-group actors to the map.',
        'config': 'jobs:\n'
                  '  - name: vcenter\n'
                  '    url: https://vcenter.example.com\n'
                  '    username: netdata@vsphere.local\n'
                  '    password: SECRET\n'
                  '    collect_network_topology: yes\n',
    }],
)

CATO_SETUP = setup_block(
    'go.d/cato_networks.conf',
    'Configure the Cato Networks collector with your account ID and API key. See the '
    '[Cato Networks collector](/src/go/plugin/go.d/collector/cato_networks/integrations/cato_networks.md) page '
    'for all options. `update_every` must be at least 60; `url` defaults to the Cato public GraphQL endpoint and '
    'only needs setting for a different region or a proxy.',
    [('Cato API access',
      'A Cato Management Application API key with read access to the account you want to map.')],
    examples=[{
        'name': 'Minimal job',
        'folding': {'enabled': False},
        'description': 'The two required fields. `url` defaults to the Cato public GraphQL endpoint, and '
                       '`update_every` must be at least 60.',
        'config': 'jobs:\n'
                  '  - name: cato\n'
                  '    account_id: "12345"\n'
                  '    api_key: SECRET\n',
    }],
)

# Netdata has no syslog listener: an OpenTelemetry Collector receives syslog and
# exports it over OTLP to the Agent's otel plugin (docs/npm/syslog/otel-collector.md).
# The plugin's own contract lives in src/crates/otel-plugin/metadata.yaml.
SYSLOG_SETUP = setup_block(
    'otel.yaml',
    'The syslog receiver itself is configured on the OpenTelemetry Collector, not in the Agent. Use `otel.yaml` only '
    'to change the Agent\'s OTLP endpoint or retention. A ready-to-use syslog pipeline is in '
    '[Syslog via the OpenTelemetry Collector](/docs/npm/syslog/otel-collector.md). '
    'The endpoint listens on loopback by default, which accepts '
    'only local senders. Running the Collector on another host means binding a non-loopback address, and an OTLP '
    'endpoint reachable off-host must be protected with TLS or mutual TLS (`endpoint.tls_cert_path`, '
    '`endpoint.tls_key_path`, and `endpoint.tls_ca_cert_path` for mTLS) plus network access controls — otherwise '
    'anyone who can reach it can inject telemetry. Note that `auth.enabled` selects the tenant; it does not '
    'authenticate the sender. Prefer keeping the Collector on the same host as the Agent.',
    [('The OpenTelemetry plugin',
      'The Netdata Agent must include the `otel` plugin, which is available on Linux and macOS. See the '
      'OpenTelemetry collector documentation for how it is enabled in each installation method.'),
     ('An OpenTelemetry Collector',
      'An OpenTelemetry Collector with a `syslog` receiver, reachable by your network devices, exporting over OTLP to '
      'the Agent\'s endpoint (`127.0.0.1:4317` by default).'),
     ('Devices pointed at it',
      'The routers, switches, and firewalls must be configured to send syslog to the collector\'s listener.')],
    options=[
        {'name': 'endpoint.path', 'description': 'OTLP/gRPC endpoint the Agent listens on. The default accepts '
                                                 'only local senders; bind a non-loopback address to accept a '
                                                 'Collector on another host, and protect it when you do.',
         'default_value': '127.0.0.1:4317', 'required': False},
        {'name': 'endpoint.tls_cert_path', 'description': 'Server TLS certificate. Set together with '
                                                          '`endpoint.tls_key_path`.',
         'default_value': '', 'required': False},
        {'name': 'endpoint.tls_key_path', 'description': 'Server TLS private key.',
         'default_value': '', 'required': False},
        {'name': 'endpoint.tls_ca_cert_path',
         'description': 'CA certificate used to verify client certificates. Setting it enables mutual TLS and '
                        'therefore also requires the server certificate and key.',
         'default_value': '', 'required': False},
    ],
    examples=[{
        'name': 'Accepting a Collector on another host',
        'folding': {'enabled': False},
        'description': 'Binds a routable address and requires client certificates, so only Collectors holding a '
                       'certificate signed by your CA can send. Without the TLS keys this endpoint would accept '
                       'telemetry from anyone who can reach it.',
        'config': 'endpoint:\n'
                  '  path: 0.0.0.0:4317\n'
                  '  tls_cert_path: /etc/netdata/ssl/otel.crt\n'
                  '  tls_key_path: /etc/netdata/ssl/otel.key\n'
                  '  tls_ca_cert_path: /etc/netdata/ssl/ca.crt\n',
    }],
)

# Traps arrive at the Agent's listener; they are not polled. Their configuration is
# go.d/snmp_traps.conf, never the polling collector's go.d/snmp.conf.
SNMP_TRAPS_SETUP = setup_block(
    'go.d/snmp_traps.conf',
    'Configure the trap listener: the address and port it binds, the SNMP versions and credentials it accepts, and '
    'the enrichment options — see the '
    '[SNMP Trap Listener](/src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md) page for '
    'the full option reference. Trap decoding itself needs no configuration: the stock trap profiles ship with '
    'Netdata.',
    [('Devices configured to send traps',
      'The devices must be configured to send SNMP traps or INFORMs to the Netdata Agent acting as the site\'s '
      'trap receiver, and the trap port must be reachable from them.'),
     ('A usable Netdata log directory',
      'Jobs that write direct journals store them under `${NETDATA_LOG_DIR}/traps/` — `/var/log/netdata` on '
      'package installs, `/opt/netdata/var/log/netdata` on static ones. Job creation fails if that directory is '
      'missing or unwritable. A job that only exports over OTLP can set `journal.enabled: false` instead.'),
     ('Permission to bind the trap port',
      'The default listener is UDP/162, a privileged port: binding it needs `CAP_NET_BIND_SERVICE` or root on Linux. '
      'Netdata packages grant this capability, so standard installations just work; hardened or custom deployments '
      'must grant it, or move the listener to an unprivileged port.')],
    examples=[{
        'name': 'Basic (SNMPv1/v2c)',
        'folding': {'enabled': False},
        'description': 'A single listener on the standard trap port, accepting any SNMPv1/v2c community. `listen` is '
                       'required: without an endpoint the job binds nothing. Restrict the allowlist for production.',
        'config': 'jobs:\n'
                  '  - name: local\n'
                  '    listen:\n'
                  '      endpoints:\n'
                  '        - protocol: udp\n'
                  '          address: 0.0.0.0\n'
                  '          port: 162\n'
                  '    versions:\n'
                  '      - v1\n'
                  '      - v2c\n',
    }],
)

TROUBLESHOOTING = {'problems': {'list': []}}
METRICS = {'folding': {'title': 'Metrics', 'enabled': False}, 'description': '', 'availability': [], 'scopes': []}


def make_entry(name, link, categories, icon, keywords, ov, plugin_name=PLUGIN_GO_D, module_name='snmp', metrics=None,
               setup=None):
    monitored_instance = {'name': name, 'link': link, 'categories': categories, 'icon_filename': icon}
    if name in PAGE_DESCRIPTIONS:
        monitored_instance['description'] = PAGE_DESCRIPTIONS[name]

    return {
        'meta': {
            'plugin_name': plugin_name,
            'module_name': module_name,
            'monitored_instance': monitored_instance,
            'keywords': keywords,
            'related_resources': {'integrations': {'list': []}},
            'info_provided_to_referring_integrations': {'description': ''},
        },
        'overview': ov,
        'setup': setup if setup is not None else SETUP,
        'troubleshooting': TROUBLESHOOTING,
        'alerts': [],
        'metrics': metrics if metrics is not None else METRICS,
    }


def metrics_block(description_md):
    """A metrics block with NO scopes, so gen_docs renders `## Metrics` followed
    by `description_md` verbatim (the template's scope-less else-branch)."""
    return {'folding': {'title': 'Metrics', 'enabled': False},
            'description': description_md, 'availability': [], 'scopes': []}


# ── Per-profile device page support ───────────────────────────────────────────

NAME_ACRONYMS = {
    'asa', 'ftd', 'srx', 'mx', 'ex', 'qfx', 'isr', 'asr', 'ucs', 'wlc', 'nx', 'vsx',
    'os', 'ios', 'pa', 'ap', 'wap', 'poe', 'ups', 'pdu', 'san', 'nas', 'vpn', 'wan',
    'lan', 'big', 'dgs', 'cbs', 'dcs', 'vsp', 'ssg', 'xgs', 'idrac', 'os10', 'sg',
}
GENERIC_NAMES = {
    'generic-device.yaml': 'Generic SNMP Device',
    'net-snmp.yaml': 'Net-SNMP Host',
    'generic-ups.yaml': 'Generic UPS (UPS-MIB)',
    'meraki-cloud-controller.yaml': 'Cisco Meraki (Cloud Controller)',
}


def _name_word(w):
    if not w:
        return w
    if w in VENDOR_SUFFIX_FIXES:
        return VENDOR_SUFFIX_FIXES[w]
    if w in NAME_ACRONYMS or len(w) <= 3:
        return w.upper()
    return w.title()


def profile_display_name(filename, vendor, dev_type):
    """Human page name for a device profile, e.g. 'Cisco ASA', 'Juniper SRX'."""
    if filename in GENERIC_NAMES:
        return GENERIC_NAMES[filename]
    base = filename[:-5] if filename.endswith('.yaml') else filename
    name = ' '.join(_name_word(w) for w in base.split('-'))
    return name


def profile_id_count(data):
    """Count of sysObjectID identifiers the profile matches (explicit + wildcard patterns)."""
    n = 0
    for sel in (data.get('selector') or []):
        if isinstance(sel, dict):
            for inc in ((sel.get('sysobjectid') or {}).get('include') or []):
                if isinstance(inc, str):
                    n += 1
    return n


def _ctx(name):
    return 'snmp.device_prof_' + name.replace('.', '_').replace(' ', '_')


def extract_profile_metrics(name, profiles):
    """Charted metrics across the resolved extends chain: list of dicts with
    family, context (alert reference), unit, scope, description."""
    chain = resolve_extends(name, profiles) | {name}
    out, seen = [], set()
    for m in sorted(chain):
        data = profiles.get(m, {}).get('data') or {}
        for block in (data.get('metrics') or []):
            is_table = bool(block.get('table'))
            tags = [t.get('tag') for t in (block.get('metric_tags') or [])
                    if t.get('tag') and not str(t.get('tag')).startswith('_')]
            scope = 'per ' + ', '.join(tags) if (is_table and tags) else ('per row' if is_table else 'device')
            syms = [block['symbol']] if isinstance(block.get('symbol'), dict) else (block.get('symbols') or [])
            for s in syms:
                if not isinstance(s, dict):
                    continue
                nm = s.get('name', '')
                if not nm or nm.startswith('_') or nm in seen:
                    continue
                seen.add(nm)
                cm = s.get('chart_meta') or {}
                out.append({
                    'family': cm.get('family') or 'Uncategorized',
                    'context': _ctx(nm),
                    'unit': cm.get('unit') or '',
                    'scope': scope,
                    'desc': re.sub(r'\s+', ' ', (cm.get('description') or '').strip()),
                })
    return out


def _mdx_safe(s):
    """Escape characters MDX 3 parses as JSX (`{` expression, `<` tag) in free text."""
    return s.replace('{', '&#123;').replace('}', '&#125;').replace('<', '&lt;').replace('|', '\\|')


def _unit_cell(unit):
    """Units use UCUM `{annotation}` notation; render as a code span so MDX never
    parses the braces."""
    return f'`{unit}`' if unit else '—'


def _family_group(family):
    """Group key for a metric: first two levels of the family path."""
    parts = [p for p in family.split('/') if p]
    return ' / '.join(parts[:2]) if parts else 'Uncategorized'


def render_profile_metrics_md(metrics, display):
    """Family-grouped per-metric markdown for a profile's `metrics.description`."""
    intro = (f'On top of the **generic SNMP baseline** (the *Generic SNMP Device* integration — interfaces, '
             f'system, IP/TCP/UDP, host resources), this {display} profile adds the metrics below. Each is '
             f'collected **only where the device exposes the matching OID** — inclusion means the profile '
             f'requests it; availability depends on the device model and software.')
    if not metrics:
        return intro + '\n\nThis profile adds no device-specific charted metrics beyond the baseline.'
    groups = {}
    for met in sorted(metrics, key=lambda x: (_family_group(x['family']), x['context'])):
        groups.setdefault(_family_group(met['family']), []).append(met)
    lines = [intro, '',
             f'**{len(metrics)} metrics** in {len(groups)} groups; each row is a chart context usable in alerts.', '',
             '| Group | Metrics |', '|---|---|']
    for g in sorted(groups):
        lines.append(f'| {g} | {len(groups[g])} |')
    lines.append('')
    for g in sorted(groups):
        lines.append(f'### {g}')
        lines.append('')
        lines.append('| Metric (chart context) | Unit | Scope | Description |')
        lines.append('|---|---|---|---|')
        for met in groups[g]:
            d = _mdx_safe(met['desc'][:130] + ('…' if len(met['desc']) > 130 else ''))
            lines.append(f"| `{met['context']}` | {_unit_cell(met['unit'])} | {_mdx_safe(met['scope'])} | {d} |")
        lines.append('')
    return '\n'.join(lines)


def build_device_modules(profiles):
    """One catalog page per concrete device profile (per profile, not per vendor),
    each listing the metrics it adds, grouped by family."""
    modules = []
    for name in sorted(n for n in profiles if is_device_catalog_profile(n)):
        data = profiles[name]['data']
        vendor = device_field(data, 'vendor') or inherited_field(resolve_extends(name, profiles), profiles, 'vendor')
        dev_type = device_field(data, 'type') or inherited_field(resolve_extends(name, profiles), profiles, 'type')
        display = profile_display_name(name, vendor, dev_type)
        ids = profile_id_count(data)
        mets = extract_profile_metrics(name, profiles)
        vkey = re.sub(r'[^a-z0-9]', '', (vendor or '').lower())
        cls = (dev_type or 'network device')
        id_hint = (f' (recognized across {ids} device identifiers)' if ids else '')
        modules.append(make_entry(
            name=display,
            link='',
            categories=[CAT_DEVICE],
            icon=icon_for(vkey) if vkey else FALLBACK_ICON,
            keywords=[w.lower() for w in display.split()] + ['snmp', cls.lower(), 'npm'],
            ov=overview(
                f'Monitor {display} ({cls.lower()}) with Netdata over SNMP. Netdata recognizes the device '
                f'automatically '
                f'by its `sysObjectID`{id_hint} and collects the metrics this profile declares — on top of the generic '
                f'SNMP baseline — with no manual OID configuration.',
                f'Netdata\'s SNMP collector matches the device to the **{name}** profile via `sysObjectID`/`sysDescr`, '
                f'then polls the OIDs it declares.',
                f'Auto-detected as {display} via sysObjectID/sysDescr.',
            ),
            metrics=metrics_block(render_profile_metrics_md(mets, display)),
        ))
    return modules


def build_capability_modules(vendors):
    """Per-vendor BGP and Licensing capability tiles. Device pages are per-profile
    (build_device_modules); these surface the BGP / Licensing categories."""
    modules = []

    # Generic BGP4-MIB entry, then per-vendor BGP entries.
    modules.append(make_entry(
        name='Generic BGP (BGP4-MIB)',
        link='',
        categories=[CAT_BGP],
        icon=FALLBACK_ICON,
        keywords=['bgp', 'bgp4-mib', 'snmp', 'routing', 'peering', 'npm'],
        ov=overview(
            'Monitor BGP peering and routing health on any device that implements the standard BGP4-MIB, over SNMP '
            'with '
            'Netdata.',
            'Netdata polls the standard BGP4-MIB peer table via SNMP and exposes per-peer state and counters.',
            'Available on any SNMP device that exposes the standard BGP4-MIB.',
        ),
    ))
    for key in sorted(k for k, v in vendors.items() if v['bgp']):
        display = vendors[key]['display']
        modules.append(make_entry(
            name=f'{display} BGP',
            link='',
            categories=[CAT_BGP],
            icon=icon_for(key),
            keywords=[key, 'bgp', 'snmp', 'routing', 'peering', 'npm'],
            ov=overview(
                f'Monitor BGP peering and routing health on {display} devices over SNMP with Netdata, using {display} '
                f'BGP '
                f'profile coverage.',
                f'Netdata polls the BGP peer tables exposed by {display} devices (vendor and standard BGP MIBs) via '
                f'SNMP.',
                f'Detected automatically for {display} devices that expose BGP MIBs.',
            ),
        ))

    for key in sorted(k for k, v in vendors.items() if v['lic']):
        display = vendors[key]['display']
        modules.append(make_entry(
            name=f'{display} Licensing',
            link='',
            categories=[CAT_LICENSING],
            icon=icon_for(key),
            keywords=[key, 'license', 'licensing', 'entitlement', 'expiry', 'snmp', 'npm'],
            ov=overview(
                f'Track license state, entitlements, and expiry on {display} devices over SNMP with Netdata.',
                f'Netdata reads {display} licensing telemetry (state, usage, and expiry timers) exposed over SNMP and '
                f'normalizes '
                f'it into per-device licensing charts and the `snmp:licenses` function.',
                f'Detected automatically for {display} devices that expose licensing telemetry.',
            ),
        ))

    return modules


def build_topology_modules():
    """Static topology catalog entries (not profile-derived).

    The SNMP discovery-method entries are produced by the snmp_topology collector;
    the rest are produced by the other topology producers (network-viewer, the
    streaming graph, vSphere, and Cato). All render as `device`-type tiles under
    the Topologies category and point to the live `topology:*` functions.
    """
    snmp_methods = [
        ('LLDP Topology', ['lldp', 'topology', 'l2', 'snmp', 'npm'],
         'Map Layer 2 neighbor links from devices that advertise LLDP (IEEE 802.1AB). Netdata\'s SNMP topology '
         'collector '
         'reads the LLDP local and remote tables and builds device-to-device links carrying chassis ID, port, system '
         'name, '
         'and management address.',
         'Netdata reads the LLDP-MIB local and remote neighbor tables over SNMP and stitches the links into the '
         '`topology:snmp` '
         'view.',
         'Discovered automatically on devices that expose the LLDP-MIB.'),
        ('CDP Topology', ['cdp', 'cisco', 'topology', 'l2', 'snmp', 'npm'],
         'Map Layer 2 neighbor links on Cisco and Cisco-compatible devices that run CDP. Netdata reads the CDP cache '
         'table '
         'and records the neighbor device ID, remote port, platform, native VLAN, and duplex.',
         'Netdata reads the Cisco `cdpCacheTable` over SNMP and adds the neighbor links to the `topology:snmp` view.',
         'Discovered automatically on Cisco devices that expose the CDP cache.'),
        ('FDB / MAC Forwarding Topology', ['fdb', 'bridge', 'mac', 'topology', 'l2', 'snmp', 'npm'],
         'Build the Layer 2 forwarding picture from switch MAC tables. Netdata reads the bridge forwarding database '
         '(BRIDGE-MIB '
         '/ Q-BRIDGE-MIB) to learn which MAC addresses are seen on which switch ports — the basis for locating '
         'endpoints.',
         'Netdata reads the BRIDGE-MIB and Q-BRIDGE-MIB forwarding tables over SNMP, with VLAN context, to map MAC '
         'addresses '
         'to switch ports.',
         'Discovered automatically on switches that expose the bridge forwarding database.'),
        ('ARP / IP Neighbor Topology', ['arp', 'ip neighbor', 'topology', 'l3', 'snmp', 'npm'],
         'Bind IP addresses to MAC addresses across the fabric. Netdata reads the IP neighbor / ARP table (IP-MIB '
         'ipNetToMediaTable) '
         'and cross-references it with switch FDB data to position endpoints by IP.',
         'Netdata reads the IP-MIB neighbor / ARP tables over SNMP and joins them with FDB data in the `topology:snmp` '
         'view.',
         'Discovered automatically on routers and switches that expose the ARP / IP neighbor table.'),
        ('STP Topology', ['stp', 'spanning tree', 'topology', 'l2', 'snmp', 'npm'],
         'See which Layer 2 links are forwarding and which are blocked. Netdata reads the Spanning Tree port table '
         '(BRIDGE-MIB '
         'dot1dStpPortTable) for port state, root bridge, and path cost.',
         'Netdata reads the BRIDGE-MIB Spanning Tree port table over SNMP to annotate L2 links with their STP state.',
         'Discovered automatically on switches that expose the STP port table.'),
        ('BGP Peering Topology', ['bgp', 'peering', 'topology', 'l3', 'routing', 'snmp', 'npm'],
         'Map routers to their BGP neighbors. Netdata reads BGP peer tables (BGP4-MIB plus vendor MIBs) and renders '
         'router-to-router '
         'peering links with remote AS and session state.',
         'Netdata reads the BGP4-MIB and vendor BGP peer tables over SNMP and renders the peering graph in the '
         '`topology:snmp` '
         'view.',
         'Discovered automatically on routers that expose BGP peer tables.'),
        ('OSPF Adjacency Topology', ['ospf', 'adjacency', 'topology', 'l3', 'routing', 'snmp', 'npm'],
         'Map OSPF adjacencies between routers. Netdata reads the OSPF neighbor table (OSPF-MIB ospfNbrTable) and '
         'renders '
         'the L3 adjacency graph.',
         'Netdata reads the OSPF-MIB neighbor table over SNMP and renders adjacencies in the `topology:snmp` view.',
         'Discovered automatically on routers that expose the OSPF neighbor table.'),
    ]

    # network-viewer.plugin builds topology:network-connections only where network-viewer.c
    # is compiled — Linux, FreeBSD and macOS (CMakeLists.txt). Windows builds
    # network-viewer-windows.c, which registers network-connections and network-protocols
    # but NOT the topology Function. Container/Kubernetes/systemd enrichment needs the
    # APPS_LOOKUP client, which is Linux-only (network-viewer-apps-lookup-client.h).
    other = [
        ('Live Network Connections', 'network-viewer.plugin', 'network-viewer', 'network.svg',
         ['network connections', 'sockets', 'processes', 'containers', 'kubernetes', 'dependencies', 'topology',
          'live', 'npm'],
         'Map application dependencies on a host. The network-viewer plugin reads the kernel\'s live socket table and '
         'draws which local process talks to which, and which remote endpoints they reach. On Linux it also attributes '
         'each process to the container, image, systemd unit, or Kubernetes pod, namespace, and workload that owns it, '
         'as far as the APPS_LOOKUP data allows — attribution is best effort, and a process it cannot resolve is still '
         'drawn.',
         'The network-viewer plugin builds the `topology:network-connections` view directly from the host\'s live '
         'socket table, with no SNMP and no instrumentation. It is available on Linux, FreeBSD, and macOS; container '
         'and Kubernetes attribution is Linux-only.',
         'Always on; observes the host\'s live network connections.',
         NETWORK_VIEWER_SETUP, ['Linux', 'FreeBSD', 'macOS'], False,
         'A single response is capped at 64 MiB. On a host with enough connections to exceed that, the request is '
         'aborted rather than truncated — group the map (by process name or container) to bring it back under the cap.',
         'Sockets are enumerated when the Function is called, not continuously in the background, so the cost is paid '
         'per request and scales with the number of open sockets and processes.',
         'The plugin needs privileged access to enumerate the sockets of every process; standard installations grant '
         'it. In a container it additionally needs the host network namespace, the host `/proc`, `SYS_ADMIN` for '
         'sibling containers, and `SYS_PTRACE` to attribute connections to processes. On macOS a non-privileged or '
         'TCC-restricted run omits protected processes; grant Full Disk Access where local policy requires it. The '
         'Function itself requires a signed-in Netdata identity in the same Space with permission to view sensitive '
         'data — it is not available anonymously.'),
        ('Netdata Streaming Topology', 'netdata', 'streaming', 'netdata.png',
         ['streaming', 'parents', 'children', 'topology', 'agents', 'npm'],
         'See how your Netdata Agents connect. The streaming topology renders the parent-child hierarchy of a Netdata '
         'deployment '
         '— which Agents stream to which Parents.',
         'Netdata builds the `topology:streaming` view from the live streaming connections between Agents and Parents.',
         'Always available; reflects the live streaming connections of the deployment.',
         STREAMING_SETUP, (), False, '', '', ''),
        ('vSphere Topology', PLUGIN_GO_D, 'vsphere', icon_for('vmware'),
         ['vsphere', 'vmware', 'vcenter', 'virtualization', 'topology', 'npm'],
         'Map VMware vSphere infrastructure. The vSphere collector renders clusters, hosts, VMs, and datastores with '
         'placement links and datastore-utilization overlays; enabling `collect_network_topology` adds the '
         'network and port-group attachments.',
         'The vSphere collector reads the vCenter inventory and renders it as a `netdata.topology.v1` graph.',
         'Built from the configured vCenter inventory.',
         VSPHERE_SETUP, (), True, '', '', ''),
        ('Cato Networks Topology', PLUGIN_GO_D, 'cato_networks', 'network-wired.svg',
         ['cato', 'sase', 'sd-wan', 'topology', 'npm'],
         'Map a Cato Networks SASE fabric. The Cato collector renders sites, sockets, and gateways with their tunnel '
         'and '
         'transport paths.',
         'The Cato collector reads the Cato Management Application over its API and renders the fabric as a '
         '`netdata.topology.v1` '
         'graph.',
         'Built from the configured Cato account.',
         CATO_SETUP, (), True, '', '', ''),
    ]

    modules = []
    # snmp_topology is InstancePolicySingle: one job, never side-by-side instances.
    for name, keywords, metrics_desc, method_desc, auto in snmp_methods:
        modules.append(make_entry(
            name=name, link='', categories=[CAT_TOPOLOGY], icon=FALLBACK_ICON,
            keywords=keywords, ov=overview(metrics_desc, method_desc, auto, multi_instance=False),
            plugin_name=PLUGIN_GO_D, module_name='snmp_topology', setup=SNMP_TOPOLOGY_SETUP))
    for (name, plugin, module, icon, keywords, metrics_desc, method_desc, auto, setup, platforms, multi,
         limits, perf, perms) in other:
        modules.append(make_entry(
            name=name, link='', categories=[CAT_TOPOLOGY], icon=icon,
            keywords=keywords,
            ov=overview(metrics_desc, method_desc, auto, platforms, multi, limits, perf, perms),
            plugin_name=plugin, module_name=module, setup=setup))
    return modules


def build_syslog_modules():
    """Static syslog catalog entry. Netdata has no native syslog listener; an
    OpenTelemetry Collector with a syslog receiver forwards device syslog over
    OTLP/gRPC to the Agent's otel plugin, which stores it as journal logs."""
    return [make_entry(
        name='Syslog from Network Devices',
        link='',
        categories=[CAT_SYSLOG],
        icon='syslog.png',
        keywords=['syslog', 'opentelemetry', 'otel', 'otlp', 'network devices', 'logs', 'npm'],
        ov=overview(
            'Ingest syslog from routers, switches, and firewalls into Netdata. An OpenTelemetry Collector with a '
            'syslog '
            'receiver parses the device syslog stream and forwards it over OTLP/gRPC to the Netdata Agent, which '
            'stores '
            'it as structured journal logs you explore and query in the Logs tab.',
            'Netdata does not listen for syslog directly. You run an OpenTelemetry Collector configured with a syslog '
            'receiver '
            'pointed at the Agent\'s OTLP/gRPC endpoint (default `127.0.0.1:4317`); the Agent\'s otel plugin writes '
            'the '
            'records to systemd-compatible journal files.',
            'Not auto-detected. Configure an OpenTelemetry Collector syslog receiver to forward to the Agent\'s OTLP '
            'endpoint.',
            # The otel plugin is built for Linux and macOS only, as a single instance
            # (src/crates/otel-plugin/metadata.yaml, CMakeLists.txt ENABLE_PLUGIN_OTEL).
            platforms=['Linux', 'macOS'],
            multi_instance=False,
        ),
        plugin_name='otel.plugin',
        module_name='otel',
        setup=SYSLOG_SETUP,
    )]


def build_trap_modules():
    """One catalog entry per trap-profile vendor, from the trap-profile catalogue.

    Netdata ships a generated SNMP trap profile per IANA enterprise (decoded MIBs,
    trap definitions, varbind tables). Each becomes a `device`-type tile under the
    SNMP Traps category, served by the snmp_traps collector.
    """
    if not TRAP_CATALOGUE.exists():
        return []
    catalogue = json.loads(TRAP_CATALOGUE.read_text(encoding='utf-8'))
    modules = []
    for slug in sorted(catalogue):
        entry = catalogue[slug]
        display = humanize_vendor(slug)
        traps = int(entry.get('trap_count', 0) or 0)
        mibs = int(entry.get('mib_count', 0) or 0)
        modules.append(make_entry(
            name=f'{display} SNMP Traps',
            link='',
            categories=[CAT_TRAPS],
            icon=icon_for(slug),
            keywords=[slug, 'snmp', 'trap', 'traps', 'inform', 'notification', 'npm'],
            ov=overview(
                f'Receive, decode, and store SNMP traps and INFORMs from {display} devices with Netdata. The bundled '
                f'{display} '
                f'trap profile decodes {_plural(traps, "trap definition")} across {_plural(mibs, "MIB")} into '
                f'structured '
                f'journal events with named, typed varbinds — searchable and filterable in the Logs tab.',
                f'Netdata\'s SNMP trap listener receives traps on UDP/162, matches them to the {display} enterprise '
                f'OID '
                f'space, and decodes the varbinds using the bundled {display} trap profile. No per-trap configuration.',
                f'Traps from {display} devices are decoded automatically once the device is pointed at the Agent\'s '
                f'trap '
                f'listener.',
            ),
            metrics=metrics_block(render_trap_coverage_md(entry, display)),
            plugin_name=PLUGIN_GO_D,
            module_name='snmp_traps',
            setup=SNMP_TRAPS_SETUP,
        ))
    return modules


def trap_profile_stats(entry):
    """Per-vendor trap coverage stats from the committed trap profile YAML, read
    in a single pass: per-MIB counts (grouped on the `MIB::` prefix), per-category
    counts, and per-severity counts. The catalogue index carries none of these
    breakdowns, so they are derived here."""
    empty = {'mibs': [], 'categories': {}, 'severities': {}}
    fname = entry.get('file')
    if not fname:
        return empty
    path = TRAP_PROFILES_DIR / fname
    if not path.exists():
        return empty
    data = YAML(typ='safe').load(path.read_text(encoding='utf-8')) or {}
    mibs, cats, sevs = {}, {}, {}
    for trap in (data.get('traps') or []):
        if not isinstance(trap, dict):
            continue
        name = trap.get('name') or ''
        mib = name.split('::', 1)[0] if '::' in name else '(unknown)'
        mibs[mib] = mibs.get(mib, 0) + 1
        cat = trap.get('category') or 'unknown'
        cats[cat] = cats.get(cat, 0) + 1
        sev = trap.get('severity') or 'unknown'
        sevs[sev] = sevs.get(sev, 0) + 1
    return {
        'mibs': sorted(mibs.items(), key=lambda kv: (-kv[1], kv[0].lower())),
        'categories': cats,
        'severities': sevs,
    }


def _ordered_counts_md(counts, order):
    """Inline `` `slug` N · `slug` N `` in the given canonical order, non-zero
    only, with any unexpected slugs appended (sorted) so nothing is silently
    dropped."""
    parts = [(k, counts[k]) for k in order if counts.get(k)]
    parts += sorted((k, v) for k, v in counts.items() if k not in order and v)
    return ' · '.join(f'`{k}` {v}' for k, v in parts)


def render_trap_coverage_md(entry, display):
    """Coverage section for a trap page: bounded severity/category summaries (the
    "what kinds of events" signal) plus a per-MIB trap-count breakdown (which MIBs
    decode, how deeply) and a few sample decoded trap names."""
    stats = trap_profile_stats(entry)
    breakdown = stats['mibs']
    samples = [s for s in (entry.get('sample_traps') or []) if s]
    traps = int(entry.get('trap_count', 0) or 0)
    nmibs = len(breakdown)
    lines = [f'Netdata decodes **{_plural(traps, "trap definition")}** from {display} across '
             f'**{_plural(nmibs, "MIB")}** into structured journal events. A trap is decoded '
             f'**only if the device actually sends it**; inclusion here means the profile can '
             f'decode it.', '']
    sev_md = _ordered_counts_md(stats['severities'], TRAP_SEVERITY_ORDER)
    cat_md = _ordered_counts_md(stats['categories'], TRAP_CATEGORY_ORDER)
    if sev_md:
        lines += [f'**By severity:** {sev_md}', '']
    if cat_md:
        lines += [f'**By category:** {cat_md}', '']
    if breakdown:
        table = ['| MIB | Trap definitions |', '|:----|----:|']
        table += [f'| `{mib}` | {cnt} |' for mib, cnt in breakdown]
        body = '\n'.join(table)
        lines += ['### Coverage by MIB', '']
        if nmibs > TRAP_FOLD_MIBS:
            lines += ['<details>',
                      f'<summary>{nmibs} MIBs decoded — show per-MIB trap counts</summary>',
                      '', body, '', '</details>', '']
        else:
            lines += [body, '']
    if samples:
        lines += ['### Sample decoded traps', '']
        lines += [f'- `{s}`' for s in samples[:5]]
        lines += ['']
    return '\n'.join(lines).strip()


def build_trap_enrichment_modules():
    """SNMP trap enrichment methods (not vendor-derived) — how Netdata adds
    source identity and context to received traps."""
    enrichment = [
        ('SNMP Trap Reverse DNS Enrichment', ['reverse dns', 'rdns', 'ptr', 'enrichment', 'traps', 'npm'],
         'Annotate each trap with the reverse-DNS (PTR) name of its source IP, emitted as `TRAP_REVERSE_DNS`, so traps '
         'from raw IP addresses become readable by hostname.',
         'Netdata performs a best-effort, cached PTR lookup on the trap source IP; results never override '
         'authoritative '
         'identity fields.',
         'Optional; enable reverse DNS in the trap listener configuration.'),
        ('SNMP Trap Node Attribution', ['vnode', 'identity', 'attribution', 'enrichment', 'traps', 'npm'],
         'Attribute each trap to the right Netdata node. When enrichment resolves the trap source to an unambiguous '
         'vnode, '
         'the trap and any profile metrics attach to that node; otherwise a bounded source label is used.',
         'Netdata matches the trap source identity against known vnodes and host scopes, falling back to a bounded '
         'source '
         'label when attribution is ambiguous.',
         'Always on; attribution uses the configured vnodes and host scopes.'),
        ('SNMP Trap Relay Source Resolution', ['relay', 'snmptrapaddress', 'source', 'enrichment', 'traps', 'npm'],
         'Recover the original device identity when traps arrive through a relay. For trusted relay CIDRs, Netdata '
         'reads '
         '`snmpTrapAddress.0` to attribute the trap to the originating device rather than the relay.',
         'Netdata trusts `snmpTrapAddress.0` only from the configured relay CIDR allowlist, then resolves the original '
         'source identity.',
         'Optional; configure the trusted relay CIDR allowlist.'),
    ]
    return [make_entry(
        name=name, link='', categories=[CAT_TRAPS], icon=FALLBACK_ICON,
        keywords=keywords, ov=overview(metrics_desc, method_desc, auto),
        plugin_name=PLUGIN_GO_D, module_name='snmp_traps', setup=SNMP_TRAPS_SETUP)
        for name, keywords, metrics_desc, method_desc, auto in enrichment]


def write_gap_report(profiles):
    """List device-profile metrics missing chart_meta (family/unit/description),
    so the profiles can be annotated. Written next to the catalogue for review."""
    rows = []
    for name in sorted(n for n in profiles if is_device_catalog_profile(n)):
        for met in extract_profile_metrics(name, profiles):
            missing = [k for k in ('family', 'unit', 'desc') if not (met[k] and met[k] != 'Uncategorized')]
            if missing:
                rows.append((name, met['context'], ','.join(missing)))
    out = OUTPUT.parent / 'metrics-metadata-gaps.txt'
    with out.open('w', encoding='utf-8') as f:
        f.write(f'# Device-profile metrics missing chart_meta — {len(rows)} of audit.\n')
        f.write('# profile\tcontext\tmissing\n')
        for r in rows:
            f.write('\t'.join(r) + '\n')
    return len(rows)


def main():
    profiles = load_profiles()
    vendors = collect_vendors(profiles)
    device = build_device_modules(profiles)
    capability = build_capability_modules(vendors)
    topology = build_topology_modules()
    syslog = build_syslog_modules()
    traps = build_trap_modules()
    trap_enrichment = build_trap_enrichment_modules()
    modules = device + capability + topology + syslog + traps + trap_enrichment

    doc = {'plugin_name': PLUGIN_GO_D, 'modules': modules}

    yaml = YAML()
    yaml.default_flow_style = False
    yaml.width = 4096
    # Indent block sequences under their key (yamllint indent-sequences + repo
    # metadata.yaml house style): `  - meta:` with mapping values at col 6.
    yaml.indent(mapping=2, sequence=4, offset=2)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with OUTPUT.open('w', encoding='utf-8') as f:
        f.write('# DO NOT EDIT THIS FILE DIRECTLY.\n')
        f.write('# It is generated by integrations/gen_npm_catalog.py from the SNMP device and trap profiles.\n')
        yaml.dump(doc, f)

    gaps = write_gap_report(profiles)
    n_bgp = sum(1 for v in vendors.values() if v['bgp']) + 1
    n_lic = sum(1 for v in vendors.values() if v['lic'])
    print(f'Wrote {OUTPUT} with {len(modules)} entries ({len(device)} device profiles, {n_bgp} bgp, {n_lic} licensing, '
          f'{len(topology)} topology, {len(syslog)} syslog, {len(traps)} trap vendors, {len(trap_enrichment)} trap '
          f'enrichment).')
    print(f'Metric-metadata gaps (metrics missing family/unit/description): {gaps} (see metrics-metadata-gaps.txt).')


if __name__ == '__main__':
    main()
