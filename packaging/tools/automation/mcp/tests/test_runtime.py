import errno
import os
import socket
import types
from pathlib import Path

import pytest
import yaml

from netdata_mcp import journal, runtime


def test_sanitize_accepts_valid_ids():
    for good in ("parent", "child-debug", "p1", "A_b-2", "x" * 64):
        assert runtime.sanitize_agent_id(good) == good


def test_sanitize_rejects_unsafe_ids():
    for bad in ("", "..", "a/b", "/etc", "-x", "_x", "a b", "x" * 65, "p.1", "../x"):
        with pytest.raises(ValueError):
            runtime.sanitize_agent_id(bad)


def test_run_dir_is_single_component_under_home():
    d = runtime.run_dir("child-debug")
    assert d == Path.home() / "opt" / "netdata-mcp" / "run" / "child-debug"


def test_run_dir_rejects_unsafe_id():
    with pytest.raises(ValueError):
        runtime.run_dir("../escape")


def test_runtime_dir_is_inside_the_run_dir():
    assert runtime.runtime_dir("child-debug") == runtime.run_dir("child-debug") / "run"


def test_runtime_socket_paths_limit_the_id_length(monkeypatch):
    # "/h/opt/netdata-mcp/run/" + id + "/run/otel-plugin/legacy-logs-4194304.sock"
    # is 64 + len(id) bytes, so ids up to 43 chars fit the 107-byte limit.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: Path("/h")))
    runtime.check_runtime_socket_paths("x" * 43)
    with pytest.raises(ValueError, match="1 chars too long"):
        runtime.check_runtime_socket_paths("x" * 44)


def test_install_bin_path():
    b = runtime.install_bin("/home/u/repos/nd")
    assert b.parts[-3:] == ("usr", "sbin", "netdata")
    assert "nd" in str(b)


def test_launch_command_shape():
    cmd = runtime.launch_command(Path("/i/usr/sbin/netdata"), 41000, Path("/r/etc/netdata.conf"))
    assert cmd == ["/i/usr/sbin/netdata", "-D", "-p", "41000", "-c", "/r/etc/netdata.conf"]


def test_generate_runtime_writes_isolated_conf(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    monkeypatch.setattr(journal, "journald_socket_present", lambda: True)
    rd, conf, _otlp, _http = runtime.generate_runtime("agent-x")
    assert rd == tmp_path / "opt" / "netdata-mcp" / "run" / "agent-x"
    for sub in ("etc", "cache", "lib", "log"):
        assert (rd / sub).is_dir()
    text = conf.read_text()
    assert "[db]" in text and "mode = ram" in text
    assert f"cache = {rd / 'cache'}" in text
    assert "bind to = 127.0.0.1" in text
    # unique, stable Cloud display name + ephemeral marker (auto-cleaned offline)
    assert "[global]" in text
    assert "hostname = mcp-agent-x" in text
    assert "is ephemeral node = yes" in text
    # config dir pinned to the run dir's etc so the otel plugin finds otel.yaml
    assert f"config = {rd / 'etc'}" in text
    # collector + daemon logs routed to the journal (journalctl-queryable)
    assert "[logs]" in text
    assert "collector = journal" in text
    assert "daemon = journal" in text


def test_generate_runtime_creates_the_runtime_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    runtime.generate_runtime("agent-x")
    assert runtime.runtime_dir("agent-x").is_dir()


def test_generate_runtime_uses_stderr_logs_without_journald(tmp_path, monkeypatch):
    # No journald socket -> route logs to stderr (never journal, whose plugin
    # layer panics if it can't connect). stderr surfaces through netdata_run_logs
    # rather than vanishing into on-disk collector.log/daemon.log.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    monkeypatch.setattr(journal, "journald_socket_present", lambda: False)
    _, conf, _otlp, _http = runtime.generate_runtime("agent-nj")
    text = conf.read_text()
    assert "[logs]" in text
    assert "collector = stderr" in text
    assert "daemon = stderr" in text
    # the panic-prone method must not be forced here (scoped to the method
    # assignments so an unrelated future conf value containing "journal" can't trip it)
    assert "collector = journal" not in text
    assert "daemon = journal" not in text
    # the [logs] choice must not disturb the rest of the conf
    assert "[db]" in text and "mode = ram" in text
    assert "hostname = mcp-agent-nj" in text


def test_generate_runtime_applies_overrides(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    _, conf, _otlp, _http = runtime.generate_runtime(
        "agent-y", overrides={"db": {"mode": "dbengine"}, "plugins": {"go.d": "no"}}
    )
    text = conf.read_text()
    assert "mode = dbengine" in text and "mode = ram" not in text  # override won
    assert "[plugins]" in text and "go.d = no" in text  # new section added


def _protocols(doc):
    return doc["receivers"]["otlp"]["protocols"]


def test_generate_runtime_writes_otel_yaml_with_isolated_base_dir_and_endpoint(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, otlp, _http = runtime.generate_runtime("agent-o")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # One base_dir pinned under the run dir; the plugin derives every per-signal
    # dir from it (per-agent isolation for both logs and traces).
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    # endpoint auto-assigned on loopback and reported back
    assert _protocols(doc)["grpc"] == {"endpoint": otlp}
    assert otlp.startswith("127.0.0.1:")
    # the OTLP/HTTP listener gets its OWN auto-assigned loopback port — never
    # the stock 4318 (a collide-and-fail-fast across parallel agents), never
    # the gRPC one
    http = _protocols(doc)["http"]
    assert http["enabled"] is True
    assert http["endpoint"].startswith("127.0.0.1:")
    assert http["endpoint"] != otlp
    # no per-signal dirs are emitted (derived), and no tuning knobs were set
    assert "logs" not in doc
    # global storage omitted (disabled) unless configured
    assert "remote_storage" not in doc


def test_generate_runtime_otel_http_endpoint_states(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # explicit host:port pins the listener and is reported back
    rd, _conf, _otlp, http = runtime.generate_runtime(
        "agent-h1", otel=runtime.OtelConfig(otlp_http_endpoint="127.0.0.1:4318")
    )
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert _protocols(doc)["http"] == {"enabled": True, "endpoint": "127.0.0.1:4318"}
    assert http == "127.0.0.1:4318"
    # "" (the tool-layer disable sentinel) serializes as enabled: false — the
    # plugin's disable — not as an empty address or an omission — and is
    # reported as None
    rd, _conf, _otlp, http = runtime.generate_runtime(
        "agent-h2", otel=runtime.OtelConfig(otlp_http_endpoint="")
    )
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert _protocols(doc)["http"] == {"enabled": False}
    assert http is None
    # auto-assigned: the reported endpoint is the one written
    rd, _conf, _otlp, http = runtime.generate_runtime("agent-h3")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert http is not None and _protocols(doc)["http"]["endpoint"] == http


def test_generate_runtime_auto_ports_never_collide(tmp_path, monkeypatch):
    # free_port() releases its socket, so successive calls may repeat a port.
    # Force every repeat: the reserved web port (5000) first, then the gRPC
    # pick again for the HTTP listener.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    picks = iter([5000, 6000, 6000, 7000])
    monkeypatch.setattr(runtime, "free_port", lambda: next(picks))
    rd, _conf, otlp, http = runtime.generate_runtime("agent-ports", reserved_ports=(5000,))
    assert (otlp, http) == ("127.0.0.1:6000", "127.0.0.1:7000")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert (_protocols(doc)["grpc"]["endpoint"], _protocols(doc)["http"]["endpoint"]) == (otlp, http)
    # a pinned gRPC endpoint is avoided too
    picks = iter([4317, 8000])
    rd, _conf, otlp, http = runtime.generate_runtime(
        "agent-ports2", otel=runtime.OtelConfig(otlp_endpoint="127.0.0.1:4317")
    )
    assert (otlp, http) == ("127.0.0.1:4317", "127.0.0.1:8000")


def test_generate_runtime_auto_grpc_port_avoids_a_pinned_http_port(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    picks = iter([4318, 9000])
    monkeypatch.setattr(runtime, "free_port", lambda: next(picks))
    _rd, _conf, otlp, http = runtime.generate_runtime(
        "agent-ports3", otel=runtime.OtelConfig(otlp_http_endpoint="127.0.0.1:4318")
    )
    assert (otlp, http) == ("127.0.0.1:9000", "127.0.0.1:4318")


@pytest.mark.parametrize("field", ["otlp_endpoint", "otlp_http_endpoint"])
def test_generate_runtime_refuses_a_pinned_endpoint_on_the_web_port(tmp_path, monkeypatch, field):
    # netdata binds its web port first, so the plugin's listener would fail
    # while the run reports the web port as the OTLP endpoint.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(**{field: "127.0.0.1:19999"})
    with pytest.raises(ValueError, match=f"{field} 127.0.0.1:19999 uses the agent's web port 19999"):
        runtime.generate_runtime("agent-clash", otel=cfg, reserved_ports=(19999,))


def test_web_port_clash_only_on_the_same_port():
    cfg = runtime.OtelConfig(otlp_endpoint="127.0.0.1:4317", otlp_http_endpoint="")
    assert runtime.web_port_clash(cfg, 19999) is None
    assert runtime.web_port_clash(cfg, None) is None
    assert runtime.web_port_clash(None, 4317) is None
    assert "otlp_endpoint 127.0.0.1:4317" in runtime.web_port_clash(cfg, 4317)
    assert runtime.pinned_ports(cfg) == {4317}


@pytest.mark.parametrize(
    "host, clashes",
    [
        ("127.0.0.1", True),
        ("0.0.0.0", True),
        ("[::]", True),
        ("[::ffff:127.0.0.1]", True),
        ("[::1]", False),
        ("127.0.0.2", False),
        # Not an IP literal: it may resolve to the web address, so it clashes.
        ("localhost", True),
    ],
)
def test_web_port_clash_only_where_the_address_may_overlap_the_web_bind(host, clashes):
    # The web server binds 127.0.0.1 only.
    cfg = runtime.OtelConfig(otlp_endpoint=f"{host}:5000")
    assert (runtime.web_port_clash(cfg, 5000) is not None) == clashes


def test_generate_runtime_reads_reserved_ports_once(tmp_path, monkeypatch):
    # A one-shot iterable must still exclude the web port from auto-assignment.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    picks = iter([19999, 9000, 9001])
    monkeypatch.setattr(runtime, "free_port", lambda: next(picks))
    _rd, _conf, otlp, http = runtime.generate_runtime("agent-once", reserved_ports=iter([19999]))
    assert (otlp, http) == ("127.0.0.1:9000", "127.0.0.1:9001")


def test_generate_runtime_portless_pinned_endpoint_still_fails_cleanly(tmp_path, monkeypatch):
    # A pinned endpoint without a numeric port reserves nothing; running out of
    # ports must still raise the intended RuntimeError, not a sort TypeError.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    monkeypatch.setattr(runtime, "free_port", lambda: 5000)
    with pytest.raises(RuntimeError, match="no free loopback port"):
        runtime.generate_runtime(
            "agent-ports4",
            reserved_ports=(5000,),
            otel=runtime.OtelConfig(otlp_endpoint="localhost:grpc"),
        )


def test_free_port_except_gives_up_instead_of_spinning(monkeypatch):
    monkeypatch.setattr(runtime, "free_port", lambda: 5000)
    with pytest.raises(RuntimeError, match="no free loopback port outside \\[5000\\]"):
        runtime.free_port_except({5000}, attempts=3)


def test_generate_runtime_otel_emits_journal_dir_when_set(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # A unique sentinel (not the plugin's default journal dir) so the assertion
    # can't be accidentally satisfied by a default.
    sentinel = "/srv/legacy-otel-fixture/v1"
    cfg = runtime.OtelConfig(journal_dir=sentinel)
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-j", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # journal_dir lands at logs.journal_dir for the read-only legacy viewer...
    assert doc["logs"]["journal_dir"] == sentinel
    # ...and base_dir stays pinned under the run dir (never the journal dir)
    assert doc["base_dir"] == str(rd / "lib" / "otel")


def test_generate_runtime_otel_omits_empty_journal_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # An empty string is "not set": omitted, not emitted as logs.journal_dir: "".
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-e", otel=runtime.OtelConfig(journal_dir=""))
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert "logs" not in doc


def test_generate_runtime_otel_emits_only_set_knobs(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(
        otlp_endpoint="127.0.0.1:4317",
        logs_rotation_max_entries=10,
        logs_retention_max_files=2,
        logs_crc_enabled=False,
    )
    rd, _conf, otlp, _http = runtime.generate_runtime("agent-k", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert otlp == "127.0.0.1:4317"  # caller endpoint wins over auto-assign
    assert _protocols(doc)["grpc"]["endpoint"] == "127.0.0.1:4317"
    # tuning lands flat under logs.* — the plugin's public schema (no dirs,
    # no wal/index nesting)
    assert doc["logs"]["rotation"]["default"] == {"max_entries": 10}
    assert doc["logs"]["retention"]["default"] == {"max_files": 2}
    assert doc["logs"]["crc_enabled"] is False
    assert "wal" not in doc["logs"]
    assert "index" not in doc["logs"]
    # untouched knobs stay out
    assert "max_file_size" not in doc["logs"]["rotation"]["default"]
    assert "compression_enabled" not in doc["logs"]
    # only logs was tuned → no traces section
    assert "traces" not in doc


def test_generate_runtime_otel_tunes_traces_only(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # Mirror of the logs-only test: tuning only traces emits a traces section and
    # NO logs section (no logs knobs, no journal_dir) — proves the symmetry.
    cfg = runtime.OtelConfig(traces_rotation_max_entries=5, traces_retention_max_files=1)
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-traces-only", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["traces"]["rotation"]["default"] == {"max_entries": 5}
    assert doc["traces"]["retention"]["default"] == {"max_files": 1}
    assert "logs" not in doc


def test_generate_runtime_otel_tunes_signals_independently(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # logs and traces tuned independently in one config; each lands in its own
    # section, neither leaks into the other.
    cfg = runtime.OtelConfig(
        logs_rotation_max_entries=20,
        traces_rotation_max_entries=10,
        traces_catalog_rotation_count=3,
    )
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-sig", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["logs"]["rotation"]["default"] == {"max_entries": 20}
    assert "catalog" not in doc["logs"]
    assert doc["traces"]["rotation"]["default"] == {"max_entries": 10}
    assert doc["traces"]["catalog"] == {"rotation_count": 3}
    # storage/auth are global, not per-signal — neither section carries them
    assert "remote_storage" not in doc["logs"] and "remote_storage" not in doc["traces"]


def test_generate_runtime_otel_omits_dirs_and_storage_by_default(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-cat")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # only base_dir is pinned; per-signal catalog/wal/index dirs are derived
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    assert "logs" not in doc
    # remote storage is omitted (disabled) unless explicitly configured
    assert "remote_storage" not in doc


def test_generate_runtime_otel_emits_catalog_tuning_without_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-cat2", otel=runtime.OtelConfig(logs_catalog_rotation_count=2))
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["logs"]["catalog"] == {"rotation_count": 2}


def test_generate_runtime_otel_enables_global_remote_storage_with_default_fs_uri(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(remote_storage_enabled=True, logs_catalog_rotation_count=2)
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-store", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # remote_storage is GLOBAL (top-level), not nested under logs
    assert doc["remote_storage"]["enabled"] is True
    # an omitted uri defaults to an isolated per-agent fs:// dir under the run dir
    assert doc["remote_storage"]["uri"] == f"fs://{rd / 'lib' / 'otel' / 'remote'}"
    assert "remote_storage" not in doc["logs"]
    assert doc["logs"]["catalog"]["rotation_count"] == 2


def test_generate_runtime_otel_honors_explicit_global_remote_storage_uri(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(remote_storage_enabled=True, remote_storage_uri="s3://bucket/prefix")
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-s3", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["remote_storage"] == {"enabled": True, "uri": "s3://bucket/prefix"}
    assert "logs" not in doc


def test_generate_runtime_otel_extra_yaml_deep_merges_and_wins(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # A first-class knob (max_file_size) plus a passthrough that (a) reaches
    # knobs with no first-class param and (b) conflicts on max_entries.
    cfg = runtime.OtelConfig(
        logs_rotation_max_file_size="1MB",
        logs_rotation_max_entries=50,
        extra_yaml=(
            "auth:\n  enabled: true\n"
            "logs:\n"
            '  ingest:\n    max_age: "30 days"\n'
            "  rotation:\n    default:\n      max_entries: 7\n"
        ),
    )
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-x", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # New sections merge in whole...
    assert doc["auth"] == {"enabled": True}
    assert doc["logs"]["ingest"] == {"max_age": "30 days"}
    # ...nested mappings merge (the sibling knob survives), and on a conflict
    # the passthrough wins.
    assert doc["logs"]["rotation"]["default"] == {"max_file_size": "1MB", "max_entries": 7}


def test_generate_runtime_otel_extra_yaml_cannot_override_pins(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # base_dir, both endpoint addresses and the HTTP enabled flag are harness
    # isolation invariants: the passthrough must not escape the per-agent run
    # dir, lie about the reported OTLP endpoints, or smuggle in an undeclared
    # HTTP bind.
    cfg = runtime.OtelConfig(
        extra_yaml=(
            "base_dir: /tmp/escape\n"
            "endpoint:\n"
            '  path: "1.2.3.4:0"\n'
            "receivers:\n"
            "  otlp:\n"
            "    protocols:\n"
            "      grpc:\n"
            '        endpoint: "1.2.3.4:1"\n'
            "        enabled: false\n"
            "        tls: {cert_file: /x.pem}\n"
            "      http:\n"
            '        endpoint: "1.2.3.4:2"\n'
            "        enabled: false\n"
            "        tls: {cert_file: /y.pem}\n"
        )
    )
    rd, _conf, otlp, http = runtime.generate_runtime("agent-pin", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    protocols = _protocols(doc)
    # Endpoints and the HTTP enabled flag are re-pinned to the harness values.
    assert protocols["grpc"]["endpoint"] == otlp
    assert protocols["http"]["endpoint"] == http
    assert protocols["http"]["enabled"] is True
    assert http.startswith("127.0.0.1:") and http != otlp
    # Everything else passes through: gRPC's enabled flag (a caller may run
    # the plugin HTTP-only, which leaves the reported OTLP/gRPC endpoint and
    # the gRPC push tools without a listener), either listener's TLS, and the
    # deprecated endpoint block (the plugin warns and uses the pinned
    # receivers value).
    assert protocols["grpc"]["enabled"] is False
    assert protocols["grpc"]["tls"] == {"cert_file": "/x.pem"}
    assert protocols["http"]["tls"] == {"cert_file": "/y.pem"}
    assert doc["endpoint"] == {"path": "1.2.3.4:0"}


def test_generate_runtime_disabled_http_section_holds_only_the_pin(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # A disabled listener's section holds only the pin, even when the
    # passthrough set an address for it.
    cfg = runtime.OtelConfig(
        otlp_http_endpoint="",
        extra_yaml="receivers: {otlp: {protocols: {http: {endpoint: '0.0.0.0:4318'}}}}\n",
    )
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-off", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert _protocols(doc)["http"] == {"enabled": False}


def test_generate_runtime_otel_extra_yaml_pins_survive_non_mapping_receivers(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # The re-pin must hold even when the passthrough replaces a level of
    # receivers.otlp.protocols with something that is not a mapping. The HTTP
    # listener is disabled here so the expected section is exactly the pins
    # (a disable must also survive the passthrough).
    for evil in (
        "receivers: null\n",
        "receivers: 42\n",
        "receivers: {otlp: [1, 2]}\n",
        "receivers: {otlp: {protocols: x}}\n",
        "receivers: {otlp: {protocols: {grpc: 1, http: null}}}\n",
        "base_dir: null\nreceivers: null\n",
    ):
        cfg = runtime.OtelConfig(otlp_http_endpoint="", extra_yaml=evil)
        rd, _conf, otlp, _http = runtime.generate_runtime("agent-nd", otel=cfg)
        doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
        assert doc["base_dir"] == str(rd / "lib" / "otel"), evil
        assert doc["receivers"] == {
            "otlp": {"protocols": {"grpc": {"endpoint": otlp}, "http": {"enabled": False}}}
        }, evil


def test_generate_runtime_otel_extra_yaml_rejects_invalid_yaml(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # Unparseable YAML (as opposed to a non-mapping) also surfaces as the same
    # ValueError contract from the runtime backstop, not a raw yaml.YAMLError.
    cfg = runtime.OtelConfig(extra_yaml="auth: [unclosed\n")
    with pytest.raises(ValueError, match="not valid YAML"):
        runtime.generate_runtime("agent-badyaml", otel=cfg)


def test_generate_runtime_otel_extra_yaml_passes_unknown_keys(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # Unknown keys pass through untouched — feeding the plugin's strict-config
    # refuse-to-start path is a supported test.
    cfg = runtime.OtelConfig(extra_yaml="some_future_option: true\n")
    rd, _conf, _otlp, _http = runtime.generate_runtime("agent-unk", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["some_future_option"] is True


def test_generate_runtime_otel_extra_yaml_rejects_non_mapping(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # The tool layer validates first; this is the runtime backstop.
    cfg = runtime.OtelConfig(extra_yaml="- just\n- a list\n")
    with pytest.raises(ValueError, match="mapping"):
        runtime.generate_runtime("agent-bad", otel=cfg)


def test_claim_env_empty_without_token():
    assert runtime.claim_env({}) == {}
    assert runtime.claim_env({"NETDATA_CLAIM_TOKEN": "   "}) == {}  # blank = unclaimed
    assert runtime.claim_env({"NETDATA_CLAIM_ROOMS": "r"}) == {}  # rooms alone is not enough


def test_claim_env_with_token_and_optionals():
    env = runtime.claim_env({
        "NETDATA_CLAIM_TOKEN": " tok ",
        "NETDATA_CLAIM_ROOMS": "room-1",
        "NETDATA_CLAIM_URL": "https://app.netdata.cloud",
        "UNRELATED": "x",
    })
    assert env == {
        "NETDATA_CLAIM_TOKEN": "tok",  # trimmed
        "NETDATA_CLAIM_ROOMS": "room-1",
        "NETDATA_CLAIM_URL": "https://app.netdata.cloud",
    }


def test_claim_env_token_only():
    assert runtime.claim_env({"NETDATA_CLAIM_TOKEN": "tok"}) == {"NETDATA_CLAIM_TOKEN": "tok"}


async def test_probe_ready_false_on_closed_port():
    assert await runtime.probe_ready(runtime.free_port(), timeout=0.5) is False


async def test_cloud_status_none_on_closed_port():
    # nothing listening -> best-effort returns (None, None), never raises
    assert await runtime.cloud_status(runtime.free_port(), timeout=0.5) == (None, None)


def test_local_opener_never_proxies():
    # built with an explicit empty ProxyHandler, so the opener carries NO proxy
    # handler at all -> loopback /api/v1/info fetches never route through HTTP_PROXY.
    import urllib.request
    assert not any(isinstance(h, urllib.request.ProxyHandler) for h in runtime._LOCAL_OPENER.handlers)


def test_cloud_status_coerces_non_bool_to_none(monkeypatch):
    # a non-bool field (e.g. unexpected API shape) must not flow through as a value
    monkeypatch.setattr(runtime, "_get_info", lambda port, timeout: {"agent-claimed": True, "aclk-available": "yes"})
    assert runtime._cloud_status_once(1234, 0.1) == (True, None)


def test_cloud_status_none_when_info_unavailable(monkeypatch):
    monkeypatch.setattr(runtime, "_get_info", lambda port, timeout: None)
    assert runtime._cloud_status_once(1234, 0.1) == (None, None)


def test_free_port_is_bindable_and_in_range():
    p = runtime.free_port()
    assert 1024 < p < 65536
    # it was free at selection time: we can bind it now
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind(("127.0.0.1", p))
    finally:
        s.close()


def test_port_unavailable_reason_detects_a_listener():
    holder = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    holder.bind(("127.0.0.1", 0))
    holder.listen()
    try:
        taken = holder.getsockname()[1]
        assert "already in use" in runtime.port_unavailable_reason(taken)
    finally:
        holder.close()
    assert runtime.port_unavailable_reason(taken) is None


@pytest.mark.parametrize("code", [errno.EACCES, errno.EPERM])
def test_port_unavailable_reason_names_a_privileged_port(monkeypatch, code):
    # Not "already in use": nothing holds the port, the bind is refused.
    class Refusing:
        def setsockopt(self, *a):
            pass

        def bind(self, addr):
            raise PermissionError(code, os.strerror(code))

        def close(self):
            pass

    fake = types.SimpleNamespace(**{k: getattr(socket, k) for k in ("AF_INET", "SOCK_STREAM", "SOL_SOCKET", "SO_REUSEADDR")})
    fake.socket = lambda *a: Refusing()
    monkeypatch.setattr(runtime, "socket", fake)
    reason = runtime.port_unavailable_reason(443)
    assert "needs privileges" in reason and "already in use" not in reason
