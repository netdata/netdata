from netdata_mcp.tools.otel_config import _endpoint_error


def test_endpoint_error_accepts_ip_port():
    assert _endpoint_error("a", "127.0.0.1:4317") is None
    assert _endpoint_error("a", "0.0.0.0:65535") is None
    assert _endpoint_error("a", "[::1]:4317") is None
    assert _endpoint_error("a", "[::ffff:127.0.0.1]:4317") is None


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
    ):
        err = _endpoint_error("a", value)
        assert err is not None and err.state == "error", value


def test_endpoint_error_rejects_missing_port():
    err = _endpoint_error("a", "127.0.0.1")
    assert err is not None and err.state == "error" and "ip:port" in err.message


def test_endpoint_error_rejects_garbage():
    assert _endpoint_error("a", "not-a-host-port") is not None
    assert _endpoint_error("a", ":") is not None
    assert _endpoint_error("a", "127.0.0.1:abc") is not None


def test_endpoint_error_rejects_port_out_of_range():
    err = _endpoint_error("a", "127.0.0.1:70000")
    assert err is not None and "range" in err.message


def test_http_endpoint_error_accepts_an_ip_and_rejects_a_hostname():
    assert _endpoint_error("a", "127.0.0.1:4318", name="otlp_http_endpoint") is None
    err = _endpoint_error("a", "localhost:4318", name="otlp_http_endpoint")
    assert err is not None and "not a hostname" in err.message and "otlp_http_endpoint" in err.message


def test_http_endpoint_error_names_the_parameter():
    # The message must say which knob is malformed once two endpoints exist.
    err = _endpoint_error("a", "not-a-host-port", name="otlp_http_endpoint")
    assert err is not None and err.state == "error" and "otlp_http_endpoint" in err.message
    err = _endpoint_error("a", "127.0.0.1:70000", name="otlp_http_endpoint")
    assert err is not None and "otlp_http_endpoint" in err.message and "range" in err.message


def test_endpoint_error_rejects_trailing_newline():
    # `$` matches before a final newline; the plugin's SocketAddr parse would not.
    assert _endpoint_error("a", "127.0.0.1:4317\n") is not None
    assert _endpoint_error("a", "127.0.0.1:4318\n", name="otlp_http_endpoint") is not None


def test_http_endpoint_error_rejects_garbage():
    assert _endpoint_error("a", "127.0.0.1", name="otlp_http_endpoint") is not None
    assert _endpoint_error("a", "http://127.0.0.1:4318", name="otlp_http_endpoint") is not None


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
