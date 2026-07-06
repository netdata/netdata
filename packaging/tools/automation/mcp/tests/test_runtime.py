import socket
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
    rd, conf, _otlp = runtime.generate_runtime("agent-x")
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


def test_generate_runtime_uses_stderr_logs_without_journald(tmp_path, monkeypatch):
    # No journald socket -> route logs to stderr (never journal, whose plugin
    # layer panics if it can't connect). stderr surfaces through netdata_run_logs
    # rather than vanishing into on-disk collector.log/daemon.log.
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    monkeypatch.setattr(journal, "journald_socket_present", lambda: False)
    _, conf, _otlp = runtime.generate_runtime("agent-nj")
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
    _, conf, _otlp = runtime.generate_runtime(
        "agent-y", overrides={"db": {"mode": "dbengine"}, "plugins": {"go.d": "no"}}
    )
    text = conf.read_text()
    assert "mode = dbengine" in text and "mode = ram" not in text  # override won
    assert "[plugins]" in text and "go.d = no" in text  # new section added


def test_generate_runtime_writes_otel_yaml_with_isolated_base_dir_and_endpoint(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, otlp = runtime.generate_runtime("agent-o")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # One base_dir pinned under the run dir; the plugin derives every per-signal
    # dir from it (per-agent isolation for both logs and traces).
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    # endpoint auto-assigned on loopback and reported back
    assert doc["endpoint"]["path"] == otlp
    assert otlp.startswith("127.0.0.1:")
    # no per-signal dirs are emitted (derived), and no tuning knobs were set
    assert "logs" not in doc
    # global storage omitted (disabled) unless configured
    assert "storage" not in doc


def test_generate_runtime_otel_emits_journal_dir_when_set(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # A unique sentinel (not the plugin's default journal dir) so the assertion
    # can't be accidentally satisfied by a default.
    sentinel = "/srv/legacy-otel-fixture/v1"
    cfg = runtime.OtelConfig(journal_dir=sentinel)
    rd, _conf, _otlp = runtime.generate_runtime("agent-j", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # journal_dir lands at logs.journal_dir for the read-only legacy viewer...
    assert doc["logs"]["journal_dir"] == sentinel
    # ...and base_dir stays pinned under the run dir (never the journal dir)
    assert doc["base_dir"] == str(rd / "lib" / "otel")


def test_generate_runtime_otel_omits_empty_journal_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # An empty string is "not set": omitted, not emitted as logs.journal_dir: "".
    rd, _conf, _otlp = runtime.generate_runtime("agent-e", otel=runtime.OtelConfig(journal_dir=""))
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert "logs" not in doc


def test_generate_runtime_otel_emits_only_set_knobs(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(
        otlp_endpoint="127.0.0.1:4317",
        logs_rotation_max_log_entries=10,
        logs_retention_max_files=2,
        logs_crc_enabled=False,
    )
    rd, _conf, otlp = runtime.generate_runtime("agent-k", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert otlp == "127.0.0.1:4317"  # caller endpoint wins over auto-assign
    assert doc["endpoint"]["path"] == "127.0.0.1:4317"
    # tuning lands flat under logs.* — the plugin's public schema (no dirs,
    # no wal/index nesting)
    assert doc["logs"]["rotation"]["default"] == {"max_log_entries": 10}
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
    cfg = runtime.OtelConfig(traces_rotation_max_log_entries=5, traces_retention_max_files=1)
    rd, _conf, _otlp = runtime.generate_runtime("agent-traces-only", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["traces"]["rotation"]["default"] == {"max_log_entries": 5}
    assert doc["traces"]["retention"]["default"] == {"max_files": 1}
    assert "logs" not in doc


def test_generate_runtime_otel_tunes_signals_independently(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # logs and traces tuned independently in one config; each lands in its own
    # section, neither leaks into the other.
    cfg = runtime.OtelConfig(
        logs_rotation_max_log_entries=20,
        traces_rotation_max_log_entries=10,
        traces_catalog_rotation_count=3,
    )
    rd, _conf, _otlp = runtime.generate_runtime("agent-sig", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["logs"]["rotation"]["default"] == {"max_log_entries": 20}
    assert "catalog" not in doc["logs"]
    assert doc["traces"]["rotation"]["default"] == {"max_log_entries": 10}
    assert doc["traces"]["catalog"] == {"rotation_count": 3}
    # storage/auth are global, not per-signal — neither section carries them
    assert "storage" not in doc["logs"] and "storage" not in doc["traces"]


def test_generate_runtime_otel_omits_dirs_and_storage_by_default(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, _otlp = runtime.generate_runtime("agent-cat")
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # only base_dir is pinned; per-signal catalog/wal/index dirs are derived
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    assert "logs" not in doc
    # remote storage is omitted (disabled) unless explicitly configured
    assert "storage" not in doc


def test_generate_runtime_otel_emits_catalog_tuning_without_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    rd, _conf, _otlp = runtime.generate_runtime("agent-cat2", otel=runtime.OtelConfig(logs_catalog_rotation_count=2))
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["logs"]["catalog"] == {"rotation_count": 2}


def test_generate_runtime_otel_enables_global_storage_with_default_fs_uri(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(storage_enabled=True, logs_catalog_rotation_count=2)
    rd, _conf, _otlp = runtime.generate_runtime("agent-store", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # storage is GLOBAL (top-level), not nested under logs
    assert doc["storage"]["enabled"] is True
    # an omitted uri defaults to an isolated per-agent fs:// dir under the run dir
    assert doc["storage"]["uri"] == f"fs://{rd / 'lib' / 'otel' / 'remote'}"
    assert "storage" not in doc["logs"]
    assert doc["logs"]["catalog"]["rotation_count"] == 2


def test_generate_runtime_otel_honors_explicit_global_storage_uri(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    cfg = runtime.OtelConfig(storage_enabled=True, storage_uri="s3://bucket/prefix")
    rd, _conf, _otlp = runtime.generate_runtime("agent-s3", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["storage"] == {"enabled": True, "uri": "s3://bucket/prefix"}
    assert "logs" not in doc


def test_generate_runtime_otel_extra_yaml_deep_merges_and_wins(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # A first-class knob (max_file_size) plus a passthrough that (a) reaches
    # knobs with no first-class param and (b) conflicts on max_log_entries.
    cfg = runtime.OtelConfig(
        logs_rotation_max_file_size="1MB",
        logs_rotation_max_log_entries=50,
        extra_yaml=(
            "auth:\n  enabled: true\n"
            "logs:\n"
            '  ingest:\n    max_age: "30 days"\n'
            "  rotation:\n    default:\n      max_log_entries: 7\n"
        ),
    )
    rd, _conf, _otlp = runtime.generate_runtime("agent-x", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    # New sections merge in whole...
    assert doc["auth"] == {"enabled": True}
    assert doc["logs"]["ingest"] == {"max_age": "30 days"}
    # ...nested mappings merge (the sibling knob survives), and on a conflict
    # the passthrough wins.
    assert doc["logs"]["rotation"]["default"] == {"max_file_size": "1MB", "max_log_entries": 7}


def test_generate_runtime_otel_extra_yaml_cannot_override_pins(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # base_dir and endpoint.path are harness isolation invariants: the
    # passthrough must not escape the per-agent run dir or lie about the
    # reported OTLP endpoint.
    cfg = runtime.OtelConfig(
        extra_yaml='base_dir: /tmp/escape\nendpoint:\n  path: "1.2.3.4:1"\n  tls_cert_path: /x.pem\n'
    )
    rd, _conf, otlp = runtime.generate_runtime("agent-pin", otel=cfg)
    doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
    assert doc["base_dir"] == str(rd / "lib" / "otel")
    assert doc["endpoint"]["path"] == otlp
    # Non-pinned endpoint siblings still pass through.
    assert doc["endpoint"]["tls_cert_path"] == "/x.pem"


def test_generate_runtime_otel_extra_yaml_pins_survive_non_mapping_endpoint(tmp_path, monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: tmp_path))
    # The re-pin must hold even when the passthrough replaces `endpoint` with
    # something that is not a mapping (the isinstance fallback branch).
    for evil in ("endpoint: null\n", "endpoint: 42\n", "endpoint: [1, 2]\n", "base_dir: null\nendpoint: null\n"):
        cfg = runtime.OtelConfig(extra_yaml=evil)
        rd, _conf, otlp = runtime.generate_runtime("agent-nd", otel=cfg)
        doc = yaml.safe_load((rd / "etc" / "otel.yaml").read_text())
        assert doc["base_dir"] == str(rd / "lib" / "otel"), evil
        assert doc["endpoint"] == {"path": otlp}, evil


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
    rd, _conf, _otlp = runtime.generate_runtime("agent-unk", otel=cfg)
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
