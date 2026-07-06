"""Runtime environment for launched agents (transport-free: no MCP imports).

Owns where a run instance's isolated, writable state lives, agent-id validation,
free-port selection, per-agent ``netdata.conf`` generation, the launch command
line, and the HTTP readiness probe.
"""

from __future__ import annotations

import asyncio
import json
import os
import re
import socket
import urllib.request
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path

import yaml

from . import journal, profiles

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
    """Caller-tunable otel-plugin options, mapped onto ``otel.yaml`` at launch.

    Every field is optional: ``None`` means "leave the plugin default" and the
    key is omitted from the generated otel.yaml (which the plugin partial-merges
    over its own stock defaults). The local storage layout is derived from a
    single ``base_dir`` that is always pinned under the run dir for per-agent
    isolation (re-pinned after the ``extra_yaml`` merge, alongside
    ``endpoint.path``). Caller-supplied paths do exist beyond the pin:
    ``storage_uri``, ``journal_dir``, and whatever ``extra_yaml`` reaches — the
    server is a localhost-only developer tool, so the caller is trusted. The
    rotation/retention knobs are the edge-case drivers (tiny thresholds force
    multi-file splits and evictions over small, deterministic corpora).
    ``journal_dir`` is a read-only path (the legacy viewer's fixture).
    """

    otlp_endpoint: str | None = None          # endpoint.path; None → auto free loopback port
    # Per-signal tuning (dirs are derived from base_dir, not set here). Each
    # signal has an independent set; an omitted knob keeps the plugin's stock
    # default for that signal. logs.* and traces.* are symmetric.
    logs_rotation_max_file_size: str | None = None      # logs.rotation.default.max_file_size
    logs_rotation_max_log_entries: int | None = None    # logs.rotation.default.max_log_entries
    logs_rotation_max_file_duration: str | None = None  # logs.rotation.default.max_file_duration
    logs_crc_enabled: bool | None = None                # logs.crc_enabled
    logs_compression_enabled: bool | None = None        # logs.compression_enabled
    logs_retention_max_files: int | None = None         # logs.retention.default.max_files
    logs_retention_max_total_size: str | None = None    # logs.retention.default.max_total_size
    logs_catalog_rotation_count: int | None = None  # logs.catalog.rotation_count; entries per catalog before it rotates+uploads
    traces_rotation_max_file_size: str | None = None      # traces.rotation.default.max_file_size
    traces_rotation_max_log_entries: int | None = None    # traces.rotation.default.max_log_entries
    traces_rotation_max_file_duration: str | None = None  # traces.rotation.default.max_file_duration
    traces_crc_enabled: bool | None = None                # traces.crc_enabled
    traces_compression_enabled: bool | None = None        # traces.compression_enabled
    traces_retention_max_files: int | None = None         # traces.retention.default.max_files
    traces_retention_max_total_size: str | None = None    # traces.retention.default.max_total_size
    traces_catalog_rotation_count: int | None = None  # traces.catalog.rotation_count; entries per catalog before it rotates+uploads
    storage_enabled: bool | None = None        # storage.enabled (GLOBAL); remote object-storage upload of SFST + catalog files
    storage_uri: str | None = None             # storage.uri (GLOBAL); opendal URI (default: per-agent fs:// dir under the run dir)
    journal_dir: str | None = None             # logs.journal_dir; former-plugin journal files for the read-only legacy-otel-logs viewer
    # Raw-YAML escape hatch: a YAML MAPPING deep-merged over the generated
    # document (passthrough wins on conflicts; nested mappings merge, any other
    # value replaces). Reaches knobs without first-class fields (auth, ingest
    # windows, retention max_age/horizon, catalog rotation_period, per-tenant
    # override blocks, startup_op_timeout) and deliberately-invalid keys for
    # strict-config refusal tests. base_dir and endpoint.path stay pinned (the
    # harness' per-agent isolation invariants) — see _otel_doc. Validated as
    # parseable YAML at the tool boundary; a semantically bad config surfaces
    # as the plugin's own refuse-to-start (that IS the test).
    extra_yaml: str | None = None


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


def _signal_tuning(
    *,
    crc_enabled: bool | None,
    compression_enabled: bool | None,
    rotation_max_file_size: str | None,
    rotation_max_log_entries: int | None,
    rotation_max_file_duration: str | None,
    retention_max_files: int | None,
    retention_max_total_size: str | None,
    catalog_rotation_count: int | None,
) -> dict:
    """Build one signal's flat tuning sub-document (``crc_enabled``,
    ``compression_enabled``, ``rotation``, ``retention``, ``catalog`` — the
    plugin's public per-signal schema) from the caller-set knobs. Returns ``{}``
    when nothing was set (the plugin then keeps its stock per-signal defaults).
    Signal-neutral: logs and traces share it.
    """
    out: dict = {}
    if crc_enabled is not None:
        out["crc_enabled"] = crc_enabled
    if compression_enabled is not None:
        out["compression_enabled"] = compression_enabled
    rotation = {
        k: v
        for k, v in (
            ("max_file_size", rotation_max_file_size),
            ("max_log_entries", rotation_max_log_entries),
            ("max_file_duration", rotation_max_file_duration),
        )
        if v is not None
    }
    if rotation:
        out["rotation"] = {"default": rotation}
    retention = {
        k: v
        for k, v in (
            ("max_files", retention_max_files),
            ("max_total_size", retention_max_total_size),
        )
        if v is not None
    }
    if retention:
        out["retention"] = {"default": retention}
    if catalog_rotation_count is not None:
        out["catalog"] = {"rotation_count": catalog_rotation_count}
    return out


def _otel_doc(cfg: OtelConfig, rd: Path, otlp_endpoint: str) -> dict:
    """The otel.yaml override document: pinned per-agent base_dir + endpoint, plus any set knobs.

    Only fields the caller set are emitted; the plugin keeps its stock defaults
    for the rest. ``base_dir`` is always pinned under the run dir, so every
    derived per-signal dir (``{base_dir}/{logs,traces}/{wal,index,catalog}``,
    ``{base_dir}/{signal}/remote-read``, ``{base_dir}/shared/seq_highwater``)
    lands in isolation — one pin isolates both signals.
    """
    base_dir = str(rd / "lib" / "otel")

    # Per-signal tuning (dirs are derived from base_dir, not set here). logs and
    # traces are symmetric and independent; each emits only the knobs the caller
    # set, leaving the plugin's stock per-signal defaults for the rest.
    logs = _signal_tuning(
        crc_enabled=cfg.logs_crc_enabled,
        compression_enabled=cfg.logs_compression_enabled,
        rotation_max_file_size=cfg.logs_rotation_max_file_size,
        rotation_max_log_entries=cfg.logs_rotation_max_log_entries,
        rotation_max_file_duration=cfg.logs_rotation_max_file_duration,
        retention_max_files=cfg.logs_retention_max_files,
        retention_max_total_size=cfg.logs_retention_max_total_size,
        catalog_rotation_count=cfg.logs_catalog_rotation_count,
    )
    traces = _signal_tuning(
        crc_enabled=cfg.traces_crc_enabled,
        compression_enabled=cfg.traces_compression_enabled,
        rotation_max_file_size=cfg.traces_rotation_max_file_size,
        rotation_max_log_entries=cfg.traces_rotation_max_log_entries,
        rotation_max_file_duration=cfg.traces_rotation_max_file_duration,
        retention_max_files=cfg.traces_retention_max_files,
        retention_max_total_size=cfg.traces_retention_max_total_size,
        catalog_rotation_count=cfg.traces_catalog_rotation_count,
    )

    # Remote object-storage upload is GLOBAL (one switch + backend for the
    # plugin). When enabled without an explicit URI, default to a per-agent
    # fs:// directory under the run dir (opendal fs = `fs://` + absolute path),
    # isolated like the local dirs, so the upload path can be exercised
    # end-to-end without external storage.
    storage: dict = {}
    if cfg.storage_enabled is not None:
        storage["enabled"] = cfg.storage_enabled
    if cfg.storage_uri is not None:
        storage["uri"] = cfg.storage_uri
    elif cfg.storage_enabled:
        storage["uri"] = f"fs://{rd / 'lib' / 'otel' / 'remote'}"

    # Read-only legacy viewer: point it at the former plugin's journal files.
    # This is the FORMER schema's `logs.journal_dir`, read by a separate tolerant
    # probe (the current `logs` SignalConfig ignores it). The plugin only reads
    # this directory. Truthy check: omit when unset or empty.
    if cfg.journal_dir:
        logs["journal_dir"] = cfg.journal_dir

    doc: dict = {"endpoint": {"path": otlp_endpoint}, "base_dir": base_dir}
    if storage:
        doc["storage"] = storage
    if logs:
        doc["logs"] = logs
    if traces:
        doc["traces"] = traces

    # Raw-YAML escape hatch (see OtelConfig.extra_yaml): deep-merge the caller's
    # mapping over the generated doc — passthrough wins — then RE-PIN base_dir
    # and endpoint.path. Those two are harness invariants (per-agent isolation;
    # the reported OTLP endpoint), not plugin knobs to reach; everything else,
    # including keys the plugin will refuse, passes through untouched.
    if cfg.extra_yaml:
        try:
            extra = yaml.safe_load(cfg.extra_yaml)
        except yaml.YAMLError as exc:
            raise ValueError(f"extra_yaml is not valid YAML: {exc}") from exc
        if extra is not None:
            if not isinstance(extra, dict):
                raise ValueError(
                    f"extra_yaml must be a YAML mapping, got {type(extra).__name__}"
                )
            doc = _deep_merge(doc, extra)
            doc["base_dir"] = base_dir
            doc.setdefault("endpoint", {})
            if isinstance(doc["endpoint"], dict):
                doc["endpoint"]["path"] = otlp_endpoint
            else:
                doc["endpoint"] = {"path": otlp_endpoint}
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
) -> tuple[Path, Path, str]:
    """Create the isolated run dir, write netdata.conf + otel.yaml; return
    ``(run_dir, conf_path, otlp_endpoint)``.

    ``overrides`` is the per-agent extension point ({section: {key: value}}),
    deep-merged over the defaults — the hook for runtime overrides (db mode,
    plugin toggles, log target, ...).

    The otel plugin (always built, see ``profiles``) reads ``otel.yaml`` from its
    user config dir. We pin ``[directories] config`` to ``<run_dir>/etc`` so the
    plugin loads the otel.yaml we generate there (netdata derives
    ``NETDATA_USER_CONFIG_DIR`` from that key and re-exports it to plugins; the
    ``-c`` flag only loads the file, not the dir). ``otlp_endpoint`` defaults to a
    free loopback port so parallel agents don't collide on 4317; it is returned
    so the caller can record where to push OTLP data.
    """
    rd = run_dir(agent_id)
    for sub in ("etc", "cache", "lib", "log"):
        (rd / sub).mkdir(parents=True, exist_ok=True)

    conf = _default_conf(agent_id, rd)
    # Pin the user config dir so the otel plugin finds the otel.yaml below.
    conf.setdefault("directories", {})["config"] = str(rd / "etc")
    for section, kv in (overrides or {}).items():
        conf.setdefault(section, {}).update(kv)
    conf_path = rd / "etc" / "netdata.conf"
    conf_path.write_text(_render_ini(conf), encoding="utf-8")

    cfg = otel or OtelConfig()
    otlp_endpoint = cfg.otlp_endpoint or f"127.0.0.1:{free_port()}"
    otel_yaml = yaml.safe_dump(_otel_doc(cfg, rd, otlp_endpoint), sort_keys=False)
    (rd / "etc" / "otel.yaml").write_text(otel_yaml, encoding="utf-8")

    return rd, conf_path, otlp_endpoint


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
