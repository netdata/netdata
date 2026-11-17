import pytest
from pydantic import ValidationError

from netdata_mcp.otel_settings import OtelSettings, endpoint_error


def test_endpoint_error_accepts_ip_port():
    for value in ("127.0.0.1:4317", "0.0.0.0:65535", "[::1]:4317", "[::ffff:127.0.0.1]:4317"):
        assert endpoint_error(value) is None, value


def test_endpoint_error_rejects_what_the_plugin_cannot_parse():
    # The plugin parses a Rust SocketAddr: no hostnames, IPv6 only in brackets.
    for value in (
        "localhost:4317",
        "::1:4317",
        "[127.0.0.1]:4317",
        "[::1:4317",
        "http://127.0.0.1:4317",
        "[fe80::1%eth0]:4317",
        "[::1[]:4317",
        "127.0.0.1:\uff11\uff12\uff13",
        "127.0.0.1",
        "not-a-host-port",
        ":",
        "127.0.0.1:abc",
        # `$` matches before a final newline; the plugin's SocketAddr parse would not.
        "127.0.0.1:4317\n",
    ):
        err = endpoint_error(value)
        assert err is not None and "ip:port" in err, value


def test_endpoint_error_rejects_port_out_of_range():
    err = endpoint_error("127.0.0.1:70000")
    assert err is not None and "range" in err


def test_settings_validate_endpoints():
    ok = OtelSettings.model_validate({"receivers": {"otlp": {"protocols": {"http": {"endpoint": "[::1]:4318"}}}}})
    assert ok.protocol("http").endpoint == "[::1]:4318"
    with pytest.raises(ValidationError) as exc:
        OtelSettings.model_validate({"receivers": {"otlp": {"protocols": {"grpc": {"endpoint": "localhost:4317"}}}}})
    assert "not a hostname" in str(exc.value)


def test_settings_refuse_unknown_and_misplaced_keys():
    for bad in (
        {"some_future_option": True},
        {"base_dir": "/x"},
        {"endpoint": {"path": "127.0.0.1:4317"}},
        {"traces": {"journal_dir": "/x"}},
        {"logs": {"rotation": {"default": {"max_file_szie": "1MB"}}}},
    ):
        with pytest.raises(ValidationError):
            OtelSettings.model_validate(bad)


def test_settings_document_keeps_only_set_keys():
    doc = {
        "auth": {"enabled": True},
        "metrics": {"interval_secs": 5},
        "remote_storage": {"enabled": True, "read_cache_max_size": "64MB", "startup_op_timeout": "5s"},
        "logs": {
            "journal_dir": "/j",
            "rotation": {"default": {"max_entries": 10}, "t1": {"max_file_size": 4096}},
            "retention": {"default": {"max_age": "1h", "horizon": "2 days"}},
            "catalog": {"rotation_count": 2, "rotation_period": "10m"},
            "ingest": {"max_age": "1d", "future_skew": "5m"},
        },
        "traces": {"crc_enabled": False, "compression_enabled": True},
        "receivers": {"otlp": {"protocols": {"grpc": {"enabled": True, "tls": {"cert_file": "/c", "key_file": "/k"}}}}},
    }
    assert OtelSettings.model_validate(doc).document() == doc


def test_settings_refuse_a_remote_uri_without_scheme():
    with pytest.raises(ValidationError):
        OtelSettings.model_validate({"remote_storage": {"uri": "/abs/path"}})


def test_extra_yaml_error_accepts_mapping_and_empty():
    from netdata_mcp.tools.otel_config import _extra_yaml_error

    assert _extra_yaml_error("a", "auth:\n  enabled: true\n") is None
    assert _extra_yaml_error("a", "") is None  # parses to None: nothing to merge
    assert _extra_yaml_error("a", "# comment only\n") is None


def test_extra_yaml_error_rejects_invalid_yaml():
    from netdata_mcp.tools.otel_config import _extra_yaml_error

    err = _extra_yaml_error("a", "auth: [unclosed\n")
    assert err is not None and err.state == "error" and "not valid YAML" in err.message


def test_extra_yaml_error_rejects_non_mapping_top_level():
    from netdata_mcp.tools.otel_config import _extra_yaml_error

    for bad in ("- a list\n", "just a string\n", "42\n"):
        err = _extra_yaml_error("a", bad)
        assert err is not None and "mapping" in err.message, bad
