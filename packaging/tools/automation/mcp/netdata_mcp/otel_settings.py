"""Typed form of the otel plugin's user ``otel.yaml`` (transport-free core).

Mirrors the plugin's user-layer override types, so every key here is a key the
plugin accepts and unknown keys are refused before launch:

- ``src/crates/otel-plugin/src/config/mod.rs`` (``ConfigOverride``)
- ``src/crates/otel-plugin/src/config/{receivers,metrics,signal}.rs``
- ``src/crates/otel-bridge/src/config.rs`` (``RotationEntry``, ``RetentionEntry``)

Two keys are left out on purpose: ``base_dir`` (pinned under the agent's run
dir for isolation) and the deprecated top-level ``endpoint`` section. Every
field is optional; an unset field keeps the stock value. Sizes take byte-size
strings ("25MB") or integers; durations take humantime strings ("7 days",
"30m"). Keep this file in lockstep with the plugin's config types.
"""

from __future__ import annotations

import ipaddress
import re

from typing import Annotated

from pydantic import BaseModel, ConfigDict, Field, StrictBool, StrictStr, field_validator

# The plugin parses listener endpoints as a Rust SocketAddr: an IP literal
# (IPv6 in brackets) and a port, never a hostname.
_HOST_PORT_RE = re.compile(r"(?:\[([0-9A-Fa-f:.]+)\]|([0-9.]+)):([0-9]{1,5})", re.ASCII)

# Strict, so a bool or numeric string is refused instead of coerced into a
# different valid value; the plugin reads counts and sizes as unsigned integers.
Count = Annotated[int, Field(ge=0, strict=True)]
Size = StrictStr | Count


def _is_ip(text: str | None, version: int) -> bool:
    try:
        return text is not None and ipaddress.ip_address(text).version == version
    except ValueError:
        return False


def endpoint_error(value: str) -> str | None:
    """Why ``value`` is not an endpoint the plugin can bind, or None."""
    m = _HOST_PORT_RE.fullmatch(value)
    if m is None or not _is_ip(m.group(1), 6) and not _is_ip(m.group(2), 4):
        return (
            "must be 'ip:port' with an IP address, not a hostname "
            f"(e.g. '127.0.0.1:4317', '[::1]:4317'), got {value!r}"
        )
    if not (1 <= int(m.group(3)) <= 65535):
        return f"port out of range (1-65535): {value!r}"
    return None


class _Strict(BaseModel):
    model_config = ConfigDict(extra="forbid")


class Tls(_Strict):
    cert_file: str | None = None
    key_file: str | None = None
    client_ca_file: str | None = Field(default=None, description="Set to require client certificates (mTLS).")


class Protocol(_Strict):
    enabled: StrictBool | None = None
    endpoint: str | None = Field(
        default=None,
        description="'ip:port'. Omit to auto-assign a free loopback port (never the stock 4317/4318).",
    )
    tls: Tls | None = None

    @field_validator("endpoint")
    @classmethod
    def _valid_endpoint(cls, value: str | None) -> str | None:
        if value is not None and (err := endpoint_error(value)) is not None:
            raise ValueError(err)
        return value


class Protocols(_Strict):
    grpc: Protocol | None = Field(default=None, description="OTLP/gRPC listener; always on unless enabled=false.")
    http: Protocol | None = Field(default=None, description=(
        "OTLP/HTTP listener; on by default here (stock is off), enabled=false turns it off."
    ))


class Otlp(_Strict):
    protocols: Protocols | None = None


class Receivers(_Strict):
    otlp: Otlp | None = None


class Auth(_Strict):
    enabled: StrictBool | None = Field(default=None, description="Tenant authentication on the OTLP receivers.")


class Metrics(_Strict):
    chart_configs_dir: str | None = None
    interval_secs: Count | None = None
    grace_period_secs: Count | None = None
    expiry_duration_secs: Count | None = None
    max_new_charts_per_request: Count | None = None


class RemoteStorage(_Strict):
    enabled: StrictBool | None = Field(default=None, description=(
        "Upload SFST + catalog files to object storage (both signals)."
    ))
    uri: str | None = Field(default=None, description=(
        "opendal URI ('fs:///abs/path', 's3://bucket/prefix'). Omitted while enabled: a per-agent fs:// dir under the"
        " run dir."
    ))
    read_cache_max_size: Size | None = Field(default=None, description="Download cache both signals share.")
    startup_op_timeout: str | None = None

    @field_validator("uri")
    @classmethod
    def _valid_uri(cls, value: str | None) -> str | None:
        if value is not None and "://" not in value:
            raise ValueError(f"must be an opendal URI like 'fs:///path' or 's3://bucket', got {value!r}")
        return value


class RotationEntry(_Strict):
    max_file_size: Size | None = None
    max_entries: Count | None = Field(default=None, description=(
        "Tiny values (e.g. 10) force multi-file splits over a small corpus."
    ))
    max_file_duration: str | None = Field(default=None, description=(
        "Max wall time a data file stays open; idle streams seal on the periodic sweep."
    ))


class RetentionEntry(_Strict):
    max_files: Count | None = None
    max_total_size: Size | None = None
    max_age: str | None = Field(default=None, description="Measured from a file's latest data timestamp.")
    horizon: str | None = Field(default=None, description=(
        "How long catalog (index) files are kept: the remote archive horizon."
    ))


class Catalog(_Strict):
    rotation_count: Count | None = Field(default=None, description=(
        "Index files recorded before a catalog file rotates and uploads; small values force catalog rotation."
    ))
    rotation_period: str | None = None


class Ingest(_Strict):
    max_age: str | None = Field(default=None, description="Oldest record timestamp accepted.")
    future_skew: str | None = Field(default=None, description="Furthest-future record timestamp accepted.")


class Signal(_Strict):
    crc_enabled: StrictBool | None = None
    compression_enabled: StrictBool | None = None
    rotation: dict[str, RotationEntry] | None = Field(default=None, description=(
        "Per tenant; the 'default' entry applies to tenants without their own."
    ))
    retention: dict[str, RetentionEntry] | None = Field(default=None, description=(
        "Per tenant; the 'default' entry applies to tenants without their own."
    ))
    catalog: Catalog | None = None
    ingest: Ingest | None = None


class LogsSignal(Signal):
    journal_dir: str | None = Field(default=None, description=(
        "Read-only journal files of the former plugin, for the legacy-otel-logs viewer."
    ))


class OtelSettings(_Strict):
    """The user ``otel.yaml``, minus ``base_dir`` and the deprecated ``endpoint``."""

    receivers: Receivers | None = None
    auth: Auth | None = None
    metrics: Metrics | None = None
    remote_storage: RemoteStorage | None = None
    logs: LogsSignal | None = None
    traces: Signal | None = None

    def protocol(self, name: str) -> Protocol | None:
        """The ``receivers.otlp.protocols.<name>`` section, if set."""
        protocols = self.receivers and self.receivers.otlp and self.receivers.otlp.protocols
        return getattr(protocols, name) if protocols else None

    def document(self) -> dict:
        """The set keys as a plain mapping, ready for YAML."""
        return self.model_dump(mode="json", exclude_none=True)
