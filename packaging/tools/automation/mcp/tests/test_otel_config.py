from netdata_mcp.tools.otel_config import _endpoint_error


def test_endpoint_error_accepts_valid_host_port():
    assert _endpoint_error("a", "127.0.0.1:4317") is None
    assert _endpoint_error("a", "localhost:1") is None
    assert _endpoint_error("a", "0.0.0.0:65535") is None


def test_endpoint_error_rejects_missing_port():
    err = _endpoint_error("a", "127.0.0.1")
    assert err is not None and err.state == "error" and "host:port" in err.message


def test_endpoint_error_rejects_garbage():
    assert _endpoint_error("a", "not-a-host-port") is not None
    assert _endpoint_error("a", ":") is not None
    assert _endpoint_error("a", "127.0.0.1:abc") is not None


def test_endpoint_error_rejects_port_out_of_range():
    err = _endpoint_error("a", "127.0.0.1:70000")
    assert err is not None and "range" in err.message


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
