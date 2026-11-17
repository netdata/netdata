"""Runtime environment for launched agents (transport-free: no MCP imports).

Owns where a run instance's isolated, writable state lives, agent-id validation,
free-port selection, per-agent ``netdata.conf`` generation, the launch command
line, and the HTTP readiness probe.
"""

from __future__ import annotations

import asyncio
import errno
import ipaddress
import json
import os
import re
import socket
import urllib.request
from collections.abc import Iterable, Mapping
from dataclasses import dataclass, field
from pathlib import Path

import yaml

from . import journal, profiles
from .otel_settings import OtelSettings

# Cloud claim credentials, read from the environment and injected into a launched
# agent's process env (never its command line). Only a non-empty token enables
# claiming; rooms/url are optional (url defaults to app.netdata.cloud agent-side).
_CLAIM_TOKEN = "NETDATA_CLAIM_TOKEN"
_CLAIM_OPTIONAL = ("NETDATA_CLAIM_ROOMS", "NETDATA_CLAIM_URL")

# agent-id becomes a filesystem path component (run/<agent-id>), so it is
# strictly validated: 1-64 chars, starts alphanumeric, then [A-Za-z0-9_-].
# This rejects "..", "/", empty, and leading "-"/"_".
_AGENT_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


def sanitize_agent_id(agent_id: str) -> str:
    """Return ``agent_id`` if path-safe, else raise ValueError."""
    if not _AGENT_ID_RE.match(agent_id):
        raise ValueError(
            f"Invalid agent id {agent_id!r}: use 1-64 chars of [A-Za-z0-9_-], "
            "starting with a letter or digit."
        )
    return agent_id


def run_dir(agent_id: str) -> Path:
    """Per-agent isolated runtime dir; validated id keeps it a single component."""
    return Path.home() / "opt" / "netdata-mcp" / "run" / sanitize_agent_id(agent_id)


# The longest socket path netdata creates under NETDATA_RUN_DIR is the otel
# plugin's legacy-logs worker socket, named after the supervisor's pid (Linux
# pid_max is at most 4194304, 7 digits). A Unix socket path holds 107 bytes.
_LONGEST_RUNTIME_SOCKET = "otel-plugin/legacy-logs-4194304.sock"
_UNIX_SOCKET_PATH_MAX = 107


def runtime_dir(agent_id: str) -> Path:
    """The agent's own netdata runtime dir, exported to it as ``NETDATA_RUN_DIR``.

    Without it netdata falls back to the host-wide ``/tmp/netdata``, where
    parallel agents can collide on plugin socket paths and other runtime files.
    """
    return run_dir(agent_id) / "run"


def check_runtime_socket_paths(agent_id: str) -> None:
    """Raise ValueError if ``agent_id`` makes netdata's socket paths overflow."""
    excess = len(os.fsencode(runtime_dir(agent_id) / _LONGEST_RUNTIME_SOCKET)) - _UNIX_SOCKET_PATH_MAX
    if excess > 0:
        raise ValueError(
            f"Agent id {agent_id!r} is {excess} chars too long on this host: netdata's sockets "
            f"under {runtime_dir(agent_id)} would exceed the {_UNIX_SOCKET_PATH_MAX}-byte "
            f"Unix socket path limit. Use at most {len(agent_id) - excess} chars."
        )


def free_port() -> int:
    """An OS-assigned free loopback TCP port.

    The kernel never hands out a port already in use, so this cannot collide
    with a running instance. There is a small TOCTOU window before netdata
    binds it; a bind failure surfaces as a `failed` run. Recovery is not
    automatic — the caller (LLM/human) starts the agent again, which picks a
    fresh port.
    """
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]
    finally:
        sock.close()


def port_unavailable_reason(port: int) -> str | None:
    """Why ``port`` cannot be bound on loopback right now, or None if it can.

    Used for a pinned (declared) port, which the kernel did not pick: a
    listener already holding it — another agent, or a survivor of a previous
    server — or a privileged port must fail the run up front with a clear
    reason instead of netdata's bind error. SO_REUSEADDR mirrors netdata's own
    bind, so a socket of a just-stopped agent lingering in TIME_WAIT does not
    count as taken.
    """
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind(("127.0.0.1", port))
        return None
    except OSError as e:
        if e.errno == errno.EADDRINUSE:
            return "is already in use on 127.0.0.1 (another agent, or a survivor of a previous MCP server?)"
        if e.errno in (errno.EACCES, errno.EPERM):
            return "needs privileges to bind (ports below 1024 do); declare a port of 1024 or above"
        return f"cannot be bound on 127.0.0.1: {e.strerror or e}"
    finally:
        sock.close()


def install_bin(worktree: str) -> Path:
    """Path to the installed netdata binary for a worktree (one install per worktree)."""
    return Path(profiles.install_prefix(worktree)) / "usr" / "sbin" / "netdata"


def _default_conf(agent_id: str, rd: Path) -> dict[str, dict[str, str]]:
    """Minimal ephemeral test-agent config: isolated dirs, ram db, loopback.

    ``hostname`` is the agent's Cloud display name — unique and stable per
    agent_id so distinct agents are distinct cloud nodes and a restart reuses the
    same one. ``is ephemeral node`` lets Cloud auto-clean the node once it goes
    offline, so stopped dev agents don't accumulate.
    """
    conf: dict[str, dict[str, str]] = {
        "global": {
            "hostname": f"mcp-{agent_id}",
            "is ephemeral node": "yes",
        },
        "db": {"mode": "ram"},
        "directories": {
            "cache": str(rd / "cache"),
            "lib": str(rd / "lib"),
            "log": str(rd / "log"),
        },
        "web": {"bind to": "127.0.0.1"},
    }
    # Route logs so the harness can read them back. On journald hosts use the
    # journal (queryable per-agent via netdata_agent_logs); elsewhere use stderr
    # so logs still surface through netdata_run_logs instead of vanishing into
    # on-disk collector.log / daemon.log.
    #
    # `collector` is load-bearing for plugins: netdata exports NETDATA_LOG_METHOD
    # (+ format/level) to the plugins it spawns from it. With `journal` the
    # otel-plugin logs via tracing-journald (SYSLOG_IDENTIFIER=otel-plugin/<worker>);
    # with `stderr` it writes to fd 2 — and netdata redirects a plugin's fd 2 to a
    # file ONLY when the collector method itself is a file (nd_log_collectors_fd),
    # so `stderr` leaves fd 2 inherited up to netdata's own stderr, which
    # run_command captures into the run buffer. `daemon` does the same for
    # netdata's own logs. Single mechanism — no launch-env wiring.
    #
    # stderr is always safe; only tracing-journald can panic if journald is
    # unreachable, which is why journal is gated on the socket being present.
    # Routing needs only the socket to *write*; reading the journal back via
    # netdata_agent_logs additionally needs journalctl (journal.usable(),
    # checked at tool registration).
    if journal.journald_socket_present():
        conf["logs"] = {"collector": "journal", "daemon": "journal"}
    else:
        conf["logs"] = {"collector": "stderr", "daemon": "stderr"}
    return conf


@dataclass
class OtelConfig:
    """The otel-plugin config an agent launches with, written as its user
    ``otel.yaml`` (the plugin merges it over its stock file).

    ``settings`` holds the caller's keys (see ``otel_settings``); unset keys keep
    the stock values. ``extra_yaml`` is a raw YAML mapping deep-merged on top, for
    configs the typed model refuses on purpose (unknown keys for strict-config
    refusal tests). After both, the harness pins what per-agent isolation needs:
    ``base_dir`` under the run dir and each listener's ``enabled`` and endpoint. An unset endpoint auto-assigns a free loopback port
    so parallel agents never collide on 4317/4318; the HTTP listener is on unless
    ``settings`` sets its ``enabled: false``. A listener set ``enabled: false``
    gets no port and reports no endpoint.
    """

    settings: OtelSettings = field(default_factory=OtelSettings)
    extra_yaml: str | None = None

    @property
    def grpc_disabled(self) -> bool:
        grpc = self.settings.protocol("grpc")
        return bool(grpc and grpc.enabled is False)

    @property
    def grpc_endpoint(self) -> str | None:
        """The pinned OTLP/gRPC endpoint, or None (auto-assigned or disabled)."""
        grpc = self.settings.protocol("grpc")
        return None if self.grpc_disabled or grpc is None else grpc.endpoint

    @property
    def http_disabled(self) -> bool:
        http = self.settings.protocol("http")
        return bool(http and http.enabled is False)

    @property
    def http_endpoint(self) -> str | None:
        """The pinned OTLP/HTTP endpoint, or None (auto-assigned or disabled)."""
        http = self.settings.protocol("http")
        return None if self.http_disabled or http is None else http.endpoint


def _deep_merge(base: dict, override: dict) -> dict:
    """Return ``base`` with ``override`` merged in: nested mappings merge
    recursively, any other value (scalars, lists) is replaced by the override.
    Neither input is mutated."""
    out = dict(base)
    for key, value in override.items():
        if isinstance(value, dict) and isinstance(out.get(key), dict):
            out[key] = _deep_merge(out[key], value)
        else:
            out[key] = value
    return out


def _otel_doc(cfg: OtelConfig, rd: Path, otlp_endpoint: str | None, otlp_http_endpoint: str | None) -> dict:
    """The user otel.yaml: the caller's settings, then ``extra_yaml``, then the pins.

    ``base_dir`` is pinned under the run dir, so every derived dir
    (``{base_dir}/{logs,traces}/{wal,index,catalog}``, the shared download cache,
    the seq high-water file) lands in isolation for both signals. Remote storage
    enabled without a URI defaults to a per-agent ``fs://`` dir, so the upload
    path runs end to end without external storage. Both endpoints arrive
    resolved: an address, or None for a disabled listener.
    """
    base_dir = str(rd / "lib" / "otel")
    doc: dict = {"base_dir": base_dir, **cfg.settings.document()}

    remote = cfg.settings.remote_storage
    if remote and remote.enabled and remote.uri is None:
        doc["remote_storage"]["uri"] = f"fs://{rd / 'lib' / 'otel' / 'remote'}"

    # The pins below must survive the passthrough: they are harness invariants
    # (isolation, the reported OTLP endpoints, no undeclared 4318 bind), not
    # plugin knobs. Everything else in extra_yaml, keys the plugin will refuse
    # included, passes through untouched. A disabled listener's section keeps
    # only its enabled pin, so it never shows an address that is not bound.
    if cfg.extra_yaml:
        try:
            extra = yaml.safe_load(cfg.extra_yaml)
        except yaml.YAMLError as exc:
            raise ValueError(f"extra_yaml is not valid YAML: {exc}") from exc
        if extra is not None:
            if not isinstance(extra, dict):
                raise ValueError(f"extra_yaml must be a YAML mapping, got {type(extra).__name__}")
            doc = _deep_merge(doc, extra)
            doc["base_dir"] = base_dir

    protocols = doc
    for key in ("receivers", "otlp", "protocols"):
        if not isinstance(protocols.get(key), dict):
            protocols[key] = {}
        protocols = protocols[key]
    for key in ("grpc", "http"):
        if not isinstance(protocols.get(key), dict):
            protocols[key] = {}
    for key, endpoint in (("grpc", otlp_endpoint), ("http", otlp_http_endpoint)):
        if endpoint:
            protocols[key]["enabled"] = True
            protocols[key]["endpoint"] = endpoint
        else:
            protocols[key]["enabled"] = False
            protocols[key].pop("endpoint", None)
    return doc

def _ini_safe(s: str) -> str:
    # Strip newlines (the section/key injection vector) and null bytes; defensive
    # ahead of exposing overrides to callers. ';', '#', '=' are intentionally NOT
    # stripped — they are valid in netdata.conf values.
    return str(s).replace("\x00", "").replace("\r", " ").replace("\n", " ")


def _render_ini(conf: dict[str, dict[str, str]]) -> str:
    lines: list[str] = []
    for section, kv in conf.items():
        lines.append(f"[{_ini_safe(section)}]")
        for key, value in kv.items():
            lines.append(f"    {_ini_safe(key)} = {_ini_safe(value)}")
        lines.append("")
    return "\n".join(lines)


def generate_runtime(
    agent_id: str,
    overrides: dict[str, dict[str, str]] | None = None,
    otel: OtelConfig | None = None,
    reserved_ports: Iterable[int] = (),
) -> tuple[Path, Path, str | None, str | None]:
    """Create the isolated run dir, write netdata.conf + otel.yaml; return
    ``(run_dir, conf_path, otlp_endpoint, otlp_http_endpoint)`` — an endpoint
    is None when the config disables that listener.

    ``overrides`` is the per-agent extension point ({section: {key: value}}),
    deep-merged over the defaults — the hook for runtime overrides (db mode,
    plugin toggles, log target, ...).

    The otel plugin (always built, see ``profiles``) reads ``otel.yaml`` from its
    user config dir. We pin ``[directories] config`` to ``<run_dir>/etc`` so the
    plugin loads the otel.yaml we generate there (netdata derives
    ``NETDATA_USER_CONFIG_DIR`` from that key and re-exports it to plugins; the
    ``-c`` flag only loads the file, not the dir). ``otlp_endpoint`` defaults to a
    free loopback port so parallel agents don't collide on 4317; the OTLP/HTTP
    listener gets its own free port for the same reason (the plugin fails fast on
    an occupied port), unless the config disables it (``http.enabled: false``)
    or pins it; ``reserved_ports`` (the web port) are never auto-assigned, and a
    pinned endpoint that overlaps one raises ``ValueError`` (``web_port_clash``).
    Both endpoints are returned so the caller can report where to push OTLP data.
    """
    rd = run_dir(agent_id)
    for sub in ("etc", "cache", "lib", "log", "run"):
        (rd / sub).mkdir(parents=True, exist_ok=True)

    conf = _default_conf(agent_id, rd)
    # Pin the user config dir so the otel plugin finds the otel.yaml below.
    conf.setdefault("directories", {})["config"] = str(rd / "etc")
    for section, kv in (overrides or {}).items():
        conf.setdefault(section, {}).update(kv)
    conf_path = rd / "etc" / "netdata.conf"
    conf_path.write_text(_render_ini(conf), encoding="utf-8")

    cfg = otel or OtelConfig()
    reserved = set(reserved_ports)
    # A pinned endpoint on the web port would lose the bind to netdata itself,
    # while the run still reports that port as the OTLP endpoint.
    for web_port in reserved:
        if (clash := web_port_clash(cfg, web_port)) is not None:
            raise ValueError(clash)
    # Every auto-assigned port must differ from the caller's (the web port),
    # from any pinned endpoint and from each other: free_port() releases its
    # socket, so two calls can return the same port, and the plugin refuses
    # to start when its two listeners collide.
    taken = reserved | pinned_ports(cfg)
    otlp_endpoint: str | None = None
    if cfg.grpc_endpoint:
        otlp_endpoint = cfg.grpc_endpoint
    elif not cfg.grpc_disabled:
        port = free_port_except(taken)
        taken.add(port)
        otlp_endpoint = f"127.0.0.1:{port}"
    otlp_http_endpoint: str | None = None
    if not cfg.http_disabled:
        otlp_http_endpoint = cfg.http_endpoint or f"127.0.0.1:{free_port_except(taken)}"
    otel_yaml = yaml.safe_dump(
        _otel_doc(cfg, rd, otlp_endpoint, otlp_http_endpoint), sort_keys=False
    )
    (rd / "etc" / "otel.yaml").write_text(otel_yaml, encoding="utf-8")

    return rd, conf_path, otlp_endpoint, otlp_http_endpoint


def pinned_ports(cfg: OtelConfig | None) -> set[int]:
    """The ports of the OTLP endpoints ``cfg`` pins (none when auto-assigned)."""
    ports = set()
    for pinned in (cfg.grpc_endpoint, cfg.http_endpoint) if cfg else ():
        if pinned and (port := _endpoint_port(pinned)) is not None:
            ports.add(port)
    return ports


def web_port_clash(cfg: OtelConfig | None, web_port: int | None) -> str | None:
    """Why a pinned OTLP endpoint in ``cfg`` cannot share ``web_port``, or None.

    The web server binds 127.0.0.1 (``_default_conf``), so only an endpoint
    whose address may overlap it clashes: 127.0.0.1 itself, its IPv4-mapped
    form, a wildcard, or a host that is not an IP literal (it may resolve to
    127.0.0.1). ``[::1]:P`` or ``127.0.0.2:P`` can share the port.
    """
    if cfg is None or web_port is None:
        return None
    for name, endpoint in (("grpc endpoint", cfg.grpc_endpoint), ("http endpoint", cfg.http_endpoint)):
        if endpoint and _endpoint_port(endpoint) == web_port and _overlaps_web_address(endpoint):
            return f"{name} {endpoint} uses the agent's web port {web_port}; pin another port"
    return None


def _overlaps_web_address(endpoint: str) -> bool:
    host = endpoint.rpartition(":")[0].strip("[]")
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        return True  # not an IP literal: assume the worst
    if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped is not None:
        ip = ip.ipv4_mapped
    return ip.is_unspecified or ip == ipaddress.IPv4Address("127.0.0.1")


def free_port_except(taken: set[int], attempts: int = 64) -> int:
    """A free loopback port not in ``taken``.

    Bounded: the kernel normally hands out a different port on the next try,
    so failing ``attempts`` times means something is wrong with the host's
    ephemeral range — fail loudly instead of spinning.
    """
    for _ in range(attempts):
        if (port := free_port()) not in taken:
            return port
    raise RuntimeError(
        f"no free loopback port outside {sorted(taken)} after {attempts} attempts"
    )


def _endpoint_port(endpoint: str) -> int | None:
    """The port of a ``host:port`` endpoint (None if it has none)."""
    _, _, port = endpoint.rpartition(":")
    return int(port) if port.isdigit() else None


def launch_command(netdata_bin: Path, port: int, conf_path: Path) -> list[str]:
    return [str(netdata_bin), "-D", "-p", str(port), "-c", str(conf_path)]


def claim_env(environ: Mapping[str, str] | None = None) -> dict[str, str]:
    """Cloud claim credentials to inject into a launched agent's env, or ``{}``.

    Read from ``environ`` (default ``os.environ``). Returns an empty dict — meaning
    "launch unclaimed" — unless a non-empty ``NETDATA_CLAIM_TOKEN`` is present.
    Rooms/url are included only when set. The caller passes the result as the
    launch ``env`` so credentials stay off the command line.
    """
    src = os.environ if environ is None else environ
    token = (src.get(_CLAIM_TOKEN) or "").strip()
    if not token:
        return {}
    out = {_CLAIM_TOKEN: token}
    for key in _CLAIM_OPTIONAL:
        value = (src.get(key) or "").strip()
        if value:
            out[key] = value
    return out


# A loopback-only opener: never route the agent probe through HTTP(S)_PROXY (which
# would both break the probe and leak the agent's /api/v1/info off-host).
_LOCAL_OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def _get_info(port: int, timeout: float) -> dict | None:
    """Fetch /api/v1/info and return the parsed JSON object, or None on any failure."""
    try:
        with _LOCAL_OPENER.open(f"http://127.0.0.1:{port}/api/v1/info", timeout=timeout) as resp:
            if resp.status != 200:
                return None
            data = json.loads(resp.read())
            return data if isinstance(data, dict) else None
    except Exception:
        return None


def _probe_once(port: int, timeout: float) -> bool:
    return _get_info(port, timeout) is not None


async def probe_ready(port: int, timeout: float = 2.0) -> bool:
    """True once the agent answers /api/v1/info with valid JSON on ``port``."""
    return await asyncio.to_thread(_probe_once, port, timeout)


def _as_bool(value: object) -> bool | None:
    # the /api/v1/info fields are booleans; coerce anything else to None rather than
    # let a stray type flow into the bool|None model fields.
    return value if isinstance(value, bool) else None


def _cloud_status_once(port: int, timeout: float) -> tuple[bool | None, bool | None]:
    d = _get_info(port, timeout)
    if d is None:
        return (None, None)
    return (_as_bool(d.get("agent-claimed")), _as_bool(d.get("aclk-available")))


async def cloud_status(port: int, timeout: float = 2.0) -> tuple[bool | None, bool | None]:
    """``(claimed, cloud_connected)`` from the agent's /api/v1/info, or ``(None, None)``.

    ``claimed`` is whether the agent has a claimed_id (set at startup);
    ``cloud_connected`` is whether ACLK is online (the node is live in the Cloud
    UI). Best-effort and never raises — this is *reported, never waited on*:
    a fresh poll typically shows ``claimed=True`` before ``cloud_connected`` flips
    true a few seconds later.
    """
    return await asyncio.to_thread(_cloud_status_once, port, timeout)
