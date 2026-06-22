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
